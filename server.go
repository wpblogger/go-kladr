package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/buaazp/fasthttprouter"
	"github.com/getsentry/sentry-go"
	"github.com/lab259/cors"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

type docDate struct {
	ID           int          `json:"id"`
	Status       int          `json:"-"`
	FullName     string       `json:"title"`
	LocalityType localityType `json:"locality_type"`
	Region       interface{}  `json:"region"`
}

type localityType struct {
	LocalityTitle string `json:"title"`
	LocalityName  string `json:"-"`
}

type region struct {
	RegionID    int    `json:"id"`
	RegionTitle string `json:"title"`
	RegionCode  int    `json:"region_code"`
}

type docList struct {
	Count    int            `json:"count"`
	Next     interface{}    `json:"next"`
	Previous interface{}    `json:"previous"`
	Result   []localityList `json:"results"`
}

type localityList struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

var (
	querySingle   string
	queryCity     string
	queryGeo      string
	queryMultiple string
	elasticURL    string
	elasticGeoURL string
	branch        string
	rowCount      int
	listRowCount  int
)

func getLocality(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	var biteBody []byte
	var docDates []docDate
	query := generateQuery(ctx)
	if len(query) > 0 {
		biteBody = sendRequest(query)
		if len(biteBody) > 0 {
			docDates = resultFromJSON(biteBody)
		}
	}
	log.Print("Remoote IP: ", ctx.RemoteIP(), "; Query ARGS: ", ctx.Request.URI().QueryArgs(), "; Find Result Count: ", len(docDates), "; Time Spent: ", time.Since(start))
	ctx.Response.Header.Set("Content-Type", "application/json")
	if len(docDates) > 0 {
		body, err := json.Marshal(docDates)
		if err != nil {
			log.Print(err)
			sentry.CaptureException(err)
		}
		fmt.Fprint(ctx, string(body))
	} else {
		fmt.Fprint(ctx, "[]")
	}
}

func getLocalityList(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	var err error
	var pageNumber int = 1
	var docList docList
	var biteBody []byte
	var regionsOnly string
	host := string(ctx.Request.Host())
	scheme := string(ctx.Request.URI().Scheme())
	realURL := string(ctx.Request.Header.Peek("X-Real-Url"))
	if len(realURL) == 0 {
		realURL = scheme + "://" + host + string(ctx.Path())
	}
	if len(string(ctx.QueryArgs().Peek("page"))) > 0 {
		pageNumber, err = strconv.Atoi(string(ctx.QueryArgs().Peek("page")))
		if err != nil {
			pageNumber = 1
		}
	}
	query := generateQuery(ctx)
	if len(query) > 0 {
		biteBody = sendRequest(query)
		if len(biteBody) > 0 {
			docList.Result, docList.Count = resultListFromJSON(biteBody)
		}
	}
	searchVal := url.QueryEscape(string(ctx.QueryArgs().Peek("search")))
	if len(string(ctx.QueryArgs().Peek("regions_only"))) > 0 {
		regionsOnly = "&regions_only=" + string(ctx.QueryArgs().Peek("regions_only"))
	}
	if len(string(ctx.QueryArgs().Peek("cities_and_regions"))) > 0 {
		regionsOnly = "&cities_and_regions=" + string(ctx.QueryArgs().Peek("cities_and_regions"))
	}
	if docList.Count > 0 {
		if pageNumber < 2 {
			docList.Previous = nil
		} else if pageNumber == 2 {
			docList.Previous = realURL + "?search=" + searchVal + regionsOnly
		} else if pageNumber > 2 {
			docList.Previous = realURL + "?page=" + strconv.Itoa(pageNumber-1) + "&search=" + searchVal + regionsOnly
		}
		if (listRowCount * pageNumber) >= docList.Count {
			docList.Next = nil
		} else {
			docList.Next = realURL + "?page=" + strconv.Itoa(pageNumber+1) + "&search=" + searchVal + regionsOnly
		}
	}
	log.Print("Remoote IP: ", ctx.RemoteIP(), "; Query ARGS: ", ctx.Request.URI().QueryArgs(), "; Find Result Count: ", docList.Count, "; Time Spent: ", time.Since(start))
	if docList.Count == 0 || (listRowCount*(pageNumber-1)) >= docList.Count {
		ctx.Error("not found", fasthttp.StatusNotFound)
	} else {
		ctx.Response.Header.Set("Content-Type", "application/json")
		body, err := json.Marshal(docList)
		if err != nil {
			log.Print(err)
			sentry.CaptureException(err)
		}
		fmt.Fprint(ctx, string(body))
	}
}

func getGeoIP(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	queryValue := string(ctx.QueryArgs().Peek("ip"))
	matched, _ := regexp.MatchString(`^(\d{1,3}\.){3}\d{1,3}$`, queryValue)
	var docDates []docDate
	if matched {
		ipInt, err := blockToInt(queryValue)
		if err != nil {
			fmt.Fprint(ctx, "{}")
			return
		}
		regex := regexp.MustCompile(`"_source"\s*:\s*{[^}]*"city":\s*"([^"]+)"[^}]*}`)
		res := regex.FindAllStringSubmatch(string(sendGeoRequest(ipInt)), -1)
		if len(res) == 0 {
			fmt.Fprint(ctx, "{}")
			return
		}
		city := res[0][1]
		if len(city) == 0 {
			fmt.Fprint(ctx, "{}")
			return
		}
		query := fmt.Sprintf(queryCity, city)
		biteBody := sendRequest(query)
		if len(biteBody) > 0 {
			docDates = resultFromJSON(biteBody)
		}
	}
	log.Print("Remoote IP: ", ctx.RemoteIP(), "; Query ARGS: ", ctx.Request.URI().QueryArgs(), "; Find Result Count: ", len(docDates), "; Time Spent: ", time.Since(start))
	ctx.Response.Header.Set("Content-Type", "application/json")
	if len(docDates) > 0 {
		body, err := json.Marshal(docDates)
		if err != nil {
			log.Print(err)
			sentry.CaptureException(err)
		}
		fmt.Fprint(ctx, string(body))
	} else {
		fmt.Fprint(ctx, "[]")
	}
}

func generateQuery(ctx *fasthttp.RequestCtx) string {
	var queryTerm string
	var queryRegionString string
	var queryRegionID string
	var queryRegionCode string
	var queryRegionExt string
	var queryRegionCount int = 0
	var querySize int = rowCount
	var queryFrom int = 0
	var queryLocalityTitle string = `"locality_title":["г","п","с","х","д","нп","п/ст","сл","снт"]`
	var queryFilters string = `,"functions":[{"filter":{"term":{"locality_title":"г"}},"weight":200},{"filter":{"term":{"locality_title":"п"}},"weight":100}]`
	var termValue string
	var termExt string
	var emptyResult bool = false
	var query string
	if len(string(ctx.QueryArgs().Peek("search"))) > 0 {
		termValue = strings.ToLower(string(ctx.QueryArgs().Peek("search")))
		//termExt = `*`
		queryLocalityTitle = `"locality_title":["край","обл","р-н","г","п","с","х","д","нп","п/ст","сл","снт"]`
		queryFilters = `,"functions":[{"filter":{"term":{"locality_title":"г"}},"weight":200},{"filter":{"term":{"locality_title":"край"}},"weight":170},{"filter":{"term":{"locality_title":"обл"}},"weight":170},{"filter":{"term":{"locality_title":"р-н"}},"weight":170},{"filter":{"term":{"locality_title":"п"}},"weight":100}]`
		querySize = listRowCount
	}
	if len(string(ctx.QueryArgs().Peek("page"))) > 0 {
		page, err := strconv.Atoi(string(ctx.QueryArgs().Peek("page")))
		if err == nil {
			queryFrom = (page - 1) * querySize
			if queryFrom < 0 {
				queryFrom = 0
			}
		}
	}
	if len(string(ctx.QueryArgs().Peek("regions_only"))) > 0 {
		if string(ctx.QueryArgs().Peek("regions_only")) == "1" {
			queryLocalityTitle = `"region_code":[0]`
		}
	}
	if len(string(ctx.QueryArgs().Peek("cities_and_regions"))) > 0 {
		if string(ctx.QueryArgs().Peek("cities_and_regions")) == "1" {
			queryLocalityTitle = `"locality_title":["край","обл","г"]`
			queryFilters = ``
		}
	}
	if len(string(ctx.QueryArgs().Peek("term"))) == 0 {
		if len(string(ctx.QueryArgs().Peek("iterm"))) > 0 {
			termValue = strings.ToLower(string(ctx.QueryArgs().Peek("iterm")))
			termExt = `*`
		}
	} else {
		termValue = strings.ToLower(string(ctx.QueryArgs().Peek("term")))
	}
	matchedTerm, _ := regexp.MatchString(`^[^\!\?\\\,\.\/\(\)\s]+$`, termValue)
	if matchedTerm {
		queryTerm = `{"wildcard":{"locality_name":"` + termExt + termValue + `*"}},`
	}
	rxp := regexp.MustCompile(`^([^\!\?\\\,\.\/\(\)]+)\s+([^\!\?\\\,\.\/\(\)]+)$`)
	rxpGroup := rxp.FindStringSubmatch(termValue)
	if len(rxpGroup) == 3 {
		queryTerm = `{"wildcard":{"locality_name":"` + rxpGroup[1] + `*"}},{"wildcard":{"locality_name":"*` + rxpGroup[2] + `*"}},`
	} else {
		termValue = strings.Replace(termValue, " ", "", -1)
		queryTerm = `{"wildcard":{"locality_name":"` + termExt + termValue + `*"}},`
	}
	matchedRID, _ := regexp.MatchString(`^\d+$`, string(ctx.QueryArgs().Peek("region_id")))
	if matchedRID {
		queryRegionID = `{"term":{"region_id":` + string(ctx.QueryArgs().Peek("region_id")) + `}}`
		queryRegionCount++
	} else if len(ctx.QueryArgs().Peek("region_id")) != 0 {
		emptyResult = true
	}
	matchedRCD, _ := regexp.MatchString(`^\d+$`, string(ctx.QueryArgs().Peek("region_code")))
	if matchedRCD {
		queryRegionCode = `{"term":{"region_code":` + string(ctx.QueryArgs().Peek("region_code")) + `}}`
		queryRegionCount++
	} else if len(ctx.QueryArgs().Peek("region_code")) != 0 {
		emptyResult = true
	}
	if queryRegionCount == 2 {
		queryRegionExt = `,`
	}
	if queryRegionCount > 0 {
		queryRegionString = `,"filter":{"bool":{"must":[` + queryRegionID + queryRegionExt + queryRegionCode + `]}}`
	}
	if !emptyResult {
		queryCount := `"size":` + strconv.Itoa(querySize) + `,"from":` + strconv.Itoa(queryFrom)
		query = fmt.Sprintf(querySingle, queryTerm, queryLocalityTitle, queryRegionString, queryFilters, queryCount)
	}
	return query
}

func sendRequest(query string) []byte {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(elasticURL + "/_search")
	req.Header.SetContentType("application/json")
	req.Header.SetConnectionClose()
	req.Header.SetMethod("POST")
	req.SetBodyString(query)
	resp := fasthttp.AcquireResponse()
	client := &fasthttp.Client{}
	client.Do(req, resp)
	if resp.StatusCode() != fasthttp.StatusOK {
		log.Print("Elastic Response Error:", string(resp.Body()))
		return []byte(``)
	}
	return resp.Body()
}

func resultFromJSON(body []byte) []docDate {
	regex := regexp.MustCompile(`"_source"\s*:\s*({[^}]*})`)
	res := regex.FindAllStringSubmatch(string(body), -1)
	var docDates []docDate
	var docDate docDate
	if len(res) > 0 {
		var jsonBody []byte
		i := 0
		for {
			if i == len(res) {
				break
			}
			jsonBody = []byte(res[i][1])
			docDate.ID = fastjson.GetInt(jsonBody, "doc_id")
			docDate.Status = fastjson.GetInt(jsonBody, "status")
			docDate.FullName = replaceFullName(fastjson.GetString(jsonBody, "full_name"))
			docDate.LocalityType.LocalityTitle = getReplace(fastjson.GetString(jsonBody, "locality_title"))
			docDate.LocalityType.LocalityName = fastjson.GetString(jsonBody, "locality_name")
			if fastjson.GetInt(jsonBody, "region_id") != 0 {
				var docDateRegion region
				docDateRegion.RegionTitle = fastjson.GetString(jsonBody, "region_title")
				docDateRegion.RegionCode = fastjson.GetInt(jsonBody, "region_code")
				docDateRegion.RegionID = fastjson.GetInt(jsonBody, "region_id")
				docDate.Region = docDateRegion
			} else {
				docDate.Region = nil
			}
			docDates = append(docDates, docDate)
			i++
		}
	}
	return docDates
}

func resultListFromJSON(body []byte) ([]localityList, int) {
	var count int = 0
	var locList []localityList
	var locData localityList
	rxCount := regexp.MustCompile(`"hits"\s*:\s*{\s*"total"\s*:\s*{\s*"value"\s*:\s*(\d+)`)
	resCount := rxCount.FindAllStringSubmatch(string(body), -1)
	if len(resCount) > 0 {
		count, _ = strconv.Atoi(resCount[0][1])
	}
	if count > 0 {
		regex := regexp.MustCompile(`"_source"\s*:\s*({[^}]*})`)
		res := regex.FindAllStringSubmatch(string(body), -1)
		if len(res) > 0 {
			var jsonBody []byte
			i := 0
			for {
				if i == len(res) {
					break
				}
				jsonBody = []byte(res[i][1])
				locData.ID = fastjson.GetInt(jsonBody, "doc_id")
				locData.Text = fastjson.GetString(jsonBody, "locality_name") + " " + getReplace(fastjson.GetString(jsonBody, "locality_title"))
				locList = append(locList, locData)
				i++
			}
		}
	}
	return locList, count
}

func replaceFullName(a string) string {
	re := regexp.MustCompile(`^(.*)\s+(г)(,.*)$`)
	if re.MatchString(a) {
		a = re.ReplaceAllString(a, "$1 город$3")
	}
	return a
}

func sendGeoRequest(addr int64) []byte {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(elasticGeoURL + "/_search")
	req.Header.SetContentType("application/json")
	req.Header.SetConnectionClose()
	req.Header.SetMethod("POST")
	query := fmt.Sprintf(queryGeo, addr, addr)
	req.SetBodyString(query)
	resp := fasthttp.AcquireResponse()
	client := &fasthttp.Client{}
	client.Do(req, resp)
	if resp.StatusCode() != fasthttp.StatusOK {
		log.Print("Elastic Response Error:", string(resp.Body()))
		return []byte(``)
	}
	return resp.Body()
}

func blockToInt(addr string) (int64, error) {
	var ipInt int64
	r := regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)
	start := r.FindStringSubmatch(addr)
	start = append(start[:0], start[1:]...)
	startRes := "1"
	for _, v := range start {
		startRes = startRes + toThreeChar(v)
	}
	ipInt, err := strconv.ParseInt(startRes, 10, 64)
	return ipInt, err
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

func getReplace(a string) string {
	replacer := strings.NewReplacer("Респ", "республика", "обл", "область", "АО", "автономный округ", "г", "город", "п", "поселок", "с", "село", "х", "хутор", "д", "деревня", "нп", "населенный пункт", "п/ст", "поселок при станции", "сл", "слобода", "снт", "садовое некоммерческое товарищество")
	b := replacer.Replace(a)
	return b
}

func getStatus(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Content-Type", "application/json")
	fmt.Fprint(ctx, `{"status":"ok"}`)
}

func getVersion(ctx *fasthttp.RequestCtx) {
	var respError string
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(elasticURL)
	req.Header.SetConnectionClose()
	req.Header.SetMethod("GET")
	resp := fasthttp.AcquireResponse()
	client := &fasthttp.Client{MaxIdleConnDuration: time.Second}
	client.Do(req, resp)
	ctx.Response.Header.Set("Content-Type", "application/json")
	if resp.Header.ContentLength() == 0 {
		respError = "No connection to Elastic"
	}
	if resp.StatusCode() == fasthttp.StatusNotFound {
		respError = "Elastic Index KLADR not available"
	}
	if resp.StatusCode() == fasthttp.StatusOK && resp.Header.ContentLength() > 0 {
		fmt.Fprint(ctx, `{"data": {"version": "`+branch+`"}, "error": {`+respError+`}}`)
	} else {
		sentry.CaptureException(errors.New(respError))
		ctx.Error(respError, fasthttp.StatusInternalServerError)
	}
}

func main() {
	listenPort := "8080"
	if len(os.Getenv("PORT")) > 0 {
		listenPort = os.Getenv("PORT")
	}
	elasticURL = `http://localhost:9200/kladr`
	if len(os.Getenv("ELASTIC")) > 0 {
		elasticURL = os.Getenv("ELASTIC")
	}
	elasticGeoURL = `http://localhost:9200/geoip`
	if len(os.Getenv("ELASTIC")) > 0 {
		elasticGeoURL = os.Getenv("ELASTIC")
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn: os.Getenv("SENTRYURL"),
	})
	if err != nil {
		log.Panic(err)
	}
	if len(os.Getenv("BRANCH")) > 0 {
		branch = os.Getenv("BRANCH")
	}
	rowCount = 15
	listRowCount = 30
	querySingle = `{"query":{"function_score":{"boost_mode":"replace","query": {"bool":{"must":[` + "%s" + `{"terms":{` + "%s" + `}}]` + "%s" + `}}` + "%s" + `}},"sort":["_score",{"status":{"order":"desc"}}],` + "%s" + `}`
	queryCity = `{"query":{"function_score":{"boost_mode":"replace","query":{"bool":{"must":[{"match":{"locality_name":"` + "%s" + `"}},{"terms":{"locality_title":["г","п","с","х","д","нп","п/ст","сл","снт"]}}]}},"functions":[{"filter":{"term":{"locality_title":"г"}},"weight":200},{"filter":{"term":{"locality_title":"п"}},"weight":100}]}},"sort":["_score",{"status":{"order":"desc"}}],"size":1,"from":0}`
	queryGeo = `{"query":{"bool":{"must":[{"match":{"country":"RU"}},{"range":{"start_ip":{"lte":` + "%d" + `}}},{"range":{"end_ip":{"gte":` + "%d" + `}}}]}},"size":1,"from":0}`
	addCORS := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
		//Debug:            true,
	})
	router := fasthttprouter.New()
	router.GET("/locality", getLocality)
	router.GET("/locality/", getLocality)
	router.GET("/api/locality", getLocality)
	router.GET("/api/locality/", getLocality)
	router.GET("/api/kladr/for_select", getLocalityList)
	router.GET("/api/kladr/for_select/", getLocalityList)
	router.GET("/api/geoip", getGeoIP)
	router.GET("/api/geoip/", getGeoIP)
	router.GET("/status", getStatus)
	router.GET("/api/system/version", getVersion)
	server := &fasthttp.Server{
		Handler:            addCORS.Handler(router.Handler),
		MaxRequestBodySize: 100 << 20,
		ReadBufferSize:     100 << 20,
	}
	log.Print("App start on port ", listenPort)
	log.Fatal(server.ListenAndServe(":" + listenPort))
}
