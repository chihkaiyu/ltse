package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	// res, err := http.Get("https://store.line.me/emojishop/product/5e4f906cd8824d19066dfc58/zh-Hant")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	res, err := os.Open("index.html")
	defer res.Close()
	// if res.StatusCode != 200 {
	// 	log.Fatalf("status code error: %d %s", res.StatusCode, res.Status)
	// }
	doc, err := goquery.NewDocumentFromReader(res)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find(".mdCMN09ImgList .mdCMN09ImgListWarp span.mdCMN09Image").Each(func(i int, s *goquery.Selection) {
		style, ok := s.Attr("style")
		if ok {
			url := style[21 : len(style)-2]
			filename := style[len(style)-9 : len(style)-2]

			resp, err := http.Get(url)
			if err != nil {
				log.Println(err)
				return
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println(err)
				return
			}

			err = ioutil.WriteFile(filename, body, 0644)
			if err != nil {
				log.Println(err)
			}
		}
	})
}
