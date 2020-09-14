package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/go-resty/resty/v2"
	_ "github.com/jackc/pgx/stdlib"
	"github.com/jmoiron/sqlx"
)

type rowDate struct {
	ID       int    `db:"id"`
	BlockIP  string `db:"ip_block"`
	City     string `db:"city"`
	Region   string `db:"region"`
	District string `db:"district"`
	Country  string `db:"country"`
}

type docDate struct {
	ID       int    `json:"doc_id"`
	StartIP  int64  `json:"start_ip"`
	EndIP    int64  `json:"end_ip"`
	City     string `json:"city"`
	Region   string `json:"region"`
	District string `json:"district"`
	Country  string `json:"country"`
}

var pgBase *sqlx.DB
var sqlRequest string

func initElastic(url string) error {
	client := resty.New()
	respCheck, err := client.R().Head(url)
	if err != nil {
		return err
	}
	if respCheck.StatusCode() == 404 {
		createIndexQuery := `{"settings":{"number_of_shards":1},"mappings":{"properties":{"doc_id":{"type":"long"},"start_ip":{"type":"long"},"end_ip":{"type":"long"},"city":{"type":"text"},"region":{"type":"text"},"district":{"type":"text"},"country":{"type":"text"}}}}`
		respCreate, err := client.R().SetHeader("Content-Type", "application/json").SetBody(createIndexQuery).Put(url)
		if err != nil {
			return err
		}
		if respCreate.StatusCode() == 200 {
			return nil
		}
	} else if respCheck.StatusCode() == 200 {
		return nil
	}
	return errors.New("Can't create index")
}

func addElasticDoc(doc docDate, url string) error {
	docByte, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	client := resty.New()
	resp, err := client.R().SetHeader("Content-Type", "application/json").SetBody(string(docByte)).Post(url + "/_doc")
	if err != nil {
		return err
	}
	if resp.StatusCode() == 201 {
		return nil
	}
	return errors.New("Can't add document to index")
}

func getData(url string, count int) {
	offset := 0
	for {
		rows := []rowDate{}
		err := pgBase.Select(&rows, sqlRequest+" limit "+strconv.Itoa(count)+" offset "+strconv.Itoa(offset))
		if err != nil {
			log.Print(err)
			break
		}
		if len(rows) == 0 {
			break
		}
		offset = offset + count
		for _, row := range rows {
			docDate := docDate{}
			docDate.StartIP, docDate.EndIP, err = blockToInt(row.BlockIP)
			if err == nil {
				docDate.ID = row.ID
				docDate.City = row.City
				docDate.Country = row.Country
				docDate.District = row.District
				docDate.Region = row.Region
				log.Print(docDate)
			} else {
				log.Print(err)
				break
			}
			err = addElasticDoc(docDate, url)
			if err != nil {
				log.Print(err)
				break
			} else {
				log.Print(docDate.StartIP, docDate.EndIP, docDate.Country)
			}
		}
	}
}

func blockToInt(addr string) (int64, int64, error) {
	var startInt int64
	var endInt int64
	var err error
	r := regexp.MustCompile(`^\s*(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[\s-]*(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\s*$`)
	block := r.FindStringSubmatch(addr)
	if len(block) == 3 {
		r = regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)
		start := r.FindStringSubmatch(block[1])
		start = append(start[:0], start[1:]...)
		end := r.FindStringSubmatch(block[2])
		end = append(end[:0], end[1:]...)
		startRes := "1"
		endRes := "1"
		for _, v := range start {
			startRes = startRes + toThreeChar(v)
		}
		for _, v := range end {
			endRes = endRes + toThreeChar(v)
		}
		startInt, err = strconv.ParseInt(startRes, 10, 64)
		if err == nil {
			endInt, err = strconv.ParseInt(endRes, 10, 64)
		}
	}
	return startInt, endInt, err
}

func toThreeChar(val string) string {
	var result string
	switch a := len(val); a {
	case 1:
		result = "00" + val
	case 2:
		result = "0" + val
	default:
		result = val
	}
	return result
}

func main() {
	var connectionString string
	if len(os.Getenv("PGCONNECT")) > 0 {
		connectionString = os.Getenv("PGCONNECT")
	} else {
		log.Panic("Env variable PGCONNECT is null\nExample: postgres://user:pass@hostname/dbname?sslmode=disable")
	}
	elasticURL := `http://localhost:9200/geoip`
	if len(os.Getenv("ELASTIC")) > 0 {
		elasticURL = os.Getenv("ELASTIC")
	}
	count := 100
	if len(os.Getenv("COUNT")) > 0 {
		countTmp, err := strconv.Atoi(os.Getenv("COUNT"))
		if err != nil {
			log.Panic("Env COUNT must by int")
		}
		if countTmp > 0 {
			count = countTmp
		}
	}
	err := initElastic(elasticURL)
	if err != nil {
		log.Panicf("Unable to Elastic establish connection: %v\n", err)
	}
	db, err := sqlx.Open("pgx", connectionString)
	if err != nil {
		log.Panicf("Unable to Postgres establish connection: %v\n", err)
	}
	pgBase = db
	sqlRequest = `SELECT id,ip_block,coalesce(city,'') as city,coalesce(region,'') as region,coalesce(district,'') as district,country FROM django_ipgeobase_ipgeobase where country notnull`
	getData(elasticURL, count)
}
