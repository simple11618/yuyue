package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

// var _ = ioutil.ReadAll
var defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.132 Safari/537.36"

//Config that struct des data read config from "cofig.go.json"
type Config struct {
	Fp        string
	Eid       string
	Messenger bool
	Sckey     string
}

//Address orderData.addressList[Address]
type Address struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	ProvinceId    int    `json:"provinceId"`
	CityId        int    `json:"cityId"`
	CountyId      int    `json:"countyId"`
	TownId        int    `json:"townId"`
	AddressDetail string `json:"addressDetail"`
	Mobile        string `json:"mobile"`
	MobileKey     string `json:"mobileKey"`
	Email         string `json:"email"`
}

//InvoiceInfo OrderData.invoiceInfo
type InvoiceInfo struct {
	InvoiceTitle       int8   `json:"invoiceTitle"`       // -1
	InvoiceContentType int8   `json:"invoiceContentType"` // 1
	InvoicePhone       string `json:"invoicePhone"`       // """
	InvoicePhoneKey    string `json:"invoicePhoneKey"`    // ""
}

//OrderData orderData API struct
type OrderData struct {
	Token       string      `json:"token"`
	AddressList []Address   `json:"addressList"`
	InvoiceInfo InvoiceInfo `json:"invoiceInfo"`
}

//SubmitResult is struct des the result of sumitOrder
type SubmitResult struct {
	// appUrl string
	OrderId    int    `json:"orderId"`
	PcUrl      string `json:"pcUrl"`
	ResultCode int    `json:"resultCode"`
	Success    bool   `json:"success"`
	TotalMoney string `json:"totalMoney"`
}

var conf Config

func init() {
	f, err := os.Open("config.go.json")
	// var conf Config
	defer f.Close()
	if err != nil {
		log.Fatal(err)
	}
	byteJSON, _ := ioutil.ReadAll(f)
	json.Unmarshal(byteJSON, &conf)
	log.SetFlags(log.LstdFlags | log.Ltime)
}

func main() {
	jar, _ := cookiejar.New(nil)
	type Job struct {
		date  time.Time
		skuID string
	}

	date1 := time.Date(2020, time.March, 23, 9, 59, 59, 250000000, time.UTC)
	date2 := time.Date(2020, time.March, 23, 19, 59, 59, 250000000, time.UTC)
	pool := []Job{
		Job{date1, "100011521400"},
		Job{date2, "100011551632"},
		Job{date2, "100006394713"},
	}
	client := &http.Client{
		Jar: jar,
	}
	var cookies []*http.Cookie
	cookies = getCookie(client)
	parsedURL, _ := url.Parse("https://itemko.jd.com/itemShowBtn")
	client.Jar.SetCookies(parsedURL, cookies)

	c := make(chan int)
	for _, job := range pool {
		go func(job Job) {
			log.Println("等待到达", job.date.String())
			for true {
				if job.date.Unix() >= time.Now().Unix() {
					log.Println("获取链接")
					requestItemPage(client, job.skuID)
					var b []byte
					for true {
						var err error
						log.Println("获取orderData")
						b, err = getInitInfo(client, job.skuID)
						if err == nil {
							break
						}
						log.Println("获取orderData,", err)
						time.Sleep(200 * time.Millisecond)
						// break
					}
					var orderData OrderData
					err := json.Unmarshal(b, &orderData)
					if err != nil {
						log.Print("解析 orderData 失败")
					}
					values := getEncodeValues(orderData, job.skuID)
					// fmt.Println(orderData)
					retry := 30
					for retry > 0 {
						retry--
						log.Println("提交订单", retry)
						bytes, err := submitOrder(client, job.skuID, values)
						var result SubmitResult
						if err == nil {
							err := json.Unmarshal(bytes, &result)
							if err != nil {
								log.Print("Unmarshal submit data error,", err)
								continue
							}
							if result.Success {
								log.Println(result)
								sendMessage(client, result.PcUrl)
								break
							}

						} else {
							log.Print("submit order error,", retry)
						}

						time.Sleep(50 * time.Millisecond)
						// break
					}
					c <- 1
					sendMessage(client, "")
				}
				// 到时间执行一次
				return
			}
		}(job)
	}

	for i := 0; i < len(pool); i++ {
		<-c
	}
	fmt.Println("All done")
}

func getCookie(r *http.Client) []*http.Cookie {
	remote := "http://127.0.0.1:8888/getCookies"
	request, _ := http.NewRequest("GET", remote, nil)
	res, _ := r.Do(request)
	return res.Cookies()
}

func getRandomNumber(max, min int) string {
	//  9999999, min = 1000000
	if max == 0 && min == 0 {
		max = 9999999
		min = 1000000
	}
	return fmt.Sprintf("%d", rand.Intn(max-min)+min)
}

func requestItemPage(c *http.Client, skuID string) {
	max := 9999999
	min := 1000000
	remote := "https://itemko.jd.com/itemShowBtn"
	// remote = "http://127.0.0.1:8888/showHeaders"
	request, _ := http.NewRequest("GET", remote, nil)
	qs := request.URL.Query()
	qs.Add("skuId", skuID)
	qs.Add("callback", fmt.Sprintf("jQuery%s", getRandomNumber(max, min)))
	qs.Add("from", "pc")
	qs.Add("_", fmt.Sprintf("%d", time.Now().Unix()*1000))
	request.URL.RawQuery = qs.Encode()
	request.Header.Add("User-Agent", defaultUserAgent)
	request.Host = "itemko.jd.com"
	request.Header.Add("Referer", "itemko.jd.com")
	retry := 100
	type JSONData struct {
		url string
	}
	for retry > 0 {
		retry--
		res, err := c.Do(request)
		if err != nil {
			log.Printf("Do request Url: %v", err)
		}
		body, _ := ioutil.ReadAll(res.Body)
		head := -1
		end := -1
		for i := 0; i < len(body); i++ {
			if body[i] == '(' {
				head = i + 1
			}
			if body[i] == ')' {
				end = i
			}

		}
		body = body[head:end]
		var jsonData JSONData
		err = json.Unmarshal(body, &jsonData)
		if err != nil {
			log.Printf("Item show Btn Unmarshal: %v", err)
		}

		if jsonData.url != "" {
			routeURL := "https:" + jsonData.url
			itemPageURL := strings.Replace(strings.Replace(routeURL, "divide", "marathon", 1), "user_routing", "captcha.html", 1)
			log.Printf("获取到抢购链接:%s, 即将访问:%s页面", routeURL, itemPageURL)

			return
		}
		log.Println("获取链接失败:", "retry:", retry)
		time.Sleep(50 * time.Millisecond)
	}
	log.Fatal("获取链接失败退出")
}

func getInitInfo(c *http.Client, skuID string) ([]byte, error) {
	log.Print("获取抢购初始化信息...")
	remote :=
		"https://marathon.jd.com/seckillnew/orderService/pc/init.action"
	// remote = "http://localhost:8888/showHeaders"

	data := url.Values{}
	data.Add("sku", skuID)
	data.Add("num", "1")
	data.Add("isModifyAddress", "false")
	req, _ := http.NewRequest(http.MethodPost, remote, bytes.NewBufferString(data.Encode()))
	req.Header.Add("User-Agent", defaultUserAgent)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "marathon.jd.com"

	resp, err := c.Do(req)
	if err != nil {
		log.Println(err, "request orderData")
		return nil, err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err, "ioRead body")
		return nil, err
	}
	return b, nil
}

func submitOrder(c *http.Client, skuID string, data url.Values) ([]byte, error) {
	remote :=
		"https://marathon.jd.com/seckillnew/orderService/pc/submitOrder.action"
	// remote = "http://127.0.0.1:8888/showHeaders"
	req, _ := http.NewRequest(http.MethodPost, remote, bytes.NewBufferString(data.Encode()))
	req.Host = "marathon.jd.com"
	req.Header.Add("Referer", fmt.Sprintf("https://marathon.jd.com/seckill/seckill.action?skuId=%s&num=%d&rid=%d", skuID, 1, time.Now().Unix()))
	req.Header.Add("User-Agent", defaultUserAgent)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	queryString := req.URL.Query()
	queryString.Add("skuId", skuID)
	req.URL.RawQuery = queryString.Encode()

	resp, err := c.Do(req)
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bytes, nil

}

func getEncodeValues(orderData OrderData, skuID string) url.Values {
	defaultAddress := orderData.AddressList[0]
	invoiceInfo := orderData.InvoiceInfo
	data := url.Values{}
	data.Add("skuId", skuID)
	data.Add("num", "1")
	data.Add("addressId", fmt.Sprintf("%d", defaultAddress.Id))
	data.Add("yuShou", "true")
	data.Add("isModifyAddress", "false")
	data.Add("name", defaultAddress.Name)
	data.Add("provinceId", fmt.Sprintf("%d", defaultAddress.ProvinceId))
	data.Add("cityId", fmt.Sprintf("%d", defaultAddress.CityId))
	data.Add("countyId", fmt.Sprintf("%d", defaultAddress.CountyId))
	data.Add("townId", fmt.Sprintf("%d", defaultAddress.TownId))
	data.Add("addressDetail", defaultAddress.AddressDetail)
	data.Add("mobile", defaultAddress.Mobile)
	data.Add("mobileKey", defaultAddress.MobileKey)
	data.Add("email", defaultAddress.Email)
	data.Add("postCode", "")
	if invoiceInfo.InvoiceTitle == 0 {
		data.Add("invoiceTitle", "-1")
	} else {
		data.Add("invoiceTitle", fmt.Sprintf("%d", invoiceInfo.InvoiceTitle))
	}
	data.Add("invoiceCompanyName", "")
	if invoiceInfo.InvoiceContentType == 0 {
		data.Add("invoiceContent", "1")
	} else {
		data.Add("invoiceContent", fmt.Sprintf("%d", invoiceInfo.InvoiceContentType))
	}
	data.Add("invoiceTaxpayerNO", "")
	data.Add("invoiceEmail", "")
	data.Add("invoicePhone", invoiceInfo.InvoicePhone)
	data.Add("invoicePhoneKey", invoiceInfo.InvoicePhoneKey)
	if invoiceInfo == (InvoiceInfo{}) {
		data.Add("invoice", "false")
	} else {
		data.Add("invoice", "true")
	}
	data.Add("password", "")
	data.Add("codTimeType", "3")
	data.Add("paymentType", "4")
	data.Add("areaCode", "")
	data.Add("overseas", "")
	data.Add("phone", "")
	data.Add("eid", conf.Eid)
	data.Add("fp", conf.Fp)
	data.Add("token", orderData.Token)
	data.Add("pru", "")
	return data
}

func sendMessage(c *http.Client, pcUrl string) {
	text := "抢购成功"
	if pcUrl == "" {
		text = "抢购失败"
	}
	remote := fmt.Sprintf("http://sc.ftqq.com/%s.send", conf.Sckey)
	query := url.Values{}
	query.Add("text", text)
	query.Add("desp", pcUrl)
	req, _ := http.NewRequest("GET", remote, nil)
	req.URL.RawQuery = query.Encode()
	c.Do(req)
}
