package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/go-resty/resty/v2"
)

type rowDate struct {
	ID      int    `db:"id"`
	Name    string `db:"name"`
	Abbr    string `db:"abbreviation"`
	Status  int    `db:"status"`
	DstID   int    `db:"district_id"`
	RegID   int    `db:"region_id"`
	RegCode int    `db:"code_region"`
}

type docDate struct {
	ID            int    `json:"doc_id"`
	Status        int    `json:"status"`
	FullName      string `json:"full_name"`
	LocalityTitle string `json:"locality_title"`
	LocalityName  string `json:"locality_name"`
	RegionID      int    `json:"region_id"`
	RegionTitle   string `json:"region_title"`
	RegionCode    int    `json:"region_code"`
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
		createIndexQuery := `{"settings":{"number_of_shards":1},"mappings":{"properties":{"doc_id":{"type":"long"},"status":{"type":"integer"},"full_name":{"type":"text"},"locality_title":{"type":"text"},"locality_name":{"type":"text"},"region_id":{"type":"integer"},"region_title":{"type":"text"},"region_code":{"type":"integer"}}}}`
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
			sRow := rowDate{}
			if row.RegID != 0 {
				err = pgBase.Get(&sRow, sqlRequest+" where id=$1", row.RegID)
				if err != nil {
					log.Print(err)
					break
				}
			}
			docDate.ID = row.ID
			docDate.Status = row.Status
			docDate.RegionID = row.RegID
			docDate.LocalityTitle = row.Abbr
			docDate.LocalityName = row.Name
			if row.RegID != 0 {
				docDate.RegionTitle = sRow.Name + " " + getReplace(sRow.Abbr)
				docDate.RegionCode = sRow.RegCode
				fullName := ``
				if row.DstID != 0 {
					fullName = getFullName(row.DstID)
				}
				docDate.FullName = docDate.RegionTitle + fullName + ", " + row.Abbr + ". " + row.Name
			} else {
				docDate.FullName = row.Abbr + ". " + row.Name
			}
			log.Print(docDate)
			err = addElasticDoc(docDate, url)
			if err != nil {
				log.Print(err)
				break
			} else {
				log.Print(docDate.FullName)
			}
		}
	}
}

func getFullName(id int) string {
	var nameArray []string
	for {
		row := rowDate{}
		err := pgBase.Get(&row, sqlRequest+" where id=$1", id)
		if err != nil {
			log.Print(err)
			break
		}
		name := ", " + row.Name + " " + getReplace(row.Abbr)
		nameArray = append([]string{name}, nameArray...)
		if row.DstID == 0 {
			break
		} else {
			id = row.DstID
		}
	}
	return strings.Join(nameArray, " ")
}

func getReplace(a string) string {
	replacer := strings.NewReplacer("обл", "область", "р-н", "район", "Респ", "республика", "г", "город")
	b := replacer.Replace(a)
	return b
}

func main() {
	var connectionString string
	if len(os.Getenv("PGCONNECT")) > 0 {
		connectionString = os.Getenv("PGCONNECT")
	} else {
		log.Panic("Env variable PGCONNECT is null\nExample: postgres://user:pass@hostname/dbname?sslmode=disable")
	}
	elasticURL := `http://localhost:9200/kladr`
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
	sqlRequest = `SELECT id,code_region,name,abbreviation,status,coalesce(district_id, 0) as district_id,coalesce(region_id, 0) as region_id FROM kladr_kladr`
	getData(elasticURL, count)
}
