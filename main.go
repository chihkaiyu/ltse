package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	slackAPI  = "https://slack.com/api"
	lineEmoji = "https://store.line.me/emojishop/product/"
	throttle  = 3
)

var (
	token   = flag.String("token", "", "token for requesting Slack API")
	emojiID = flag.String("id", "", "e.g., the last section of https://store.line.me/emojishop/product/5e4f906cd8824d19066dfc58")
	prefix  = flag.String("prefix", "", "prefix of Slack emoji code")
)

func init() {
	rand.Seed(time.Now().Unix())
}

type listEmojiRes struct {
	Ok    bool              `json:"ok"`
	Emoji map[string]string `json:"emoji"`
}

type uploadEmojiRes struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

func getCurrentEmojis() *listEmojiRes {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/emoji.list", slackAPI), nil)
	if err != nil {
		log.Fatal(err)
	}

	q := req.URL.Query()
	q.Add("token", *token)
	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	emojiList := &listEmojiRes{}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(data, emojiList)
	if err != nil {
		log.Fatal(err)
	}

	if !emojiList.Ok {
		log.Fatal("get emoji list failed")
	}

	return emojiList
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func prepareDir() string {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	emojiDir := filepath.Join(root, fmt.Sprintf("emojis_%03d", rand.Intn(100)))
	if exists(emojiDir) {
		err := os.RemoveAll(emojiDir)
		if err != nil {
			log.Fatal(err)
		}
	}

	err = os.MkdirAll(emojiDir, 0766)
	if err != nil {
		log.Fatal(err)
	}

	return emojiDir
}

func cleanDir(emojiDir string) {
	if err := os.RemoveAll(emojiDir); err != nil {
		log.Println(err)
	}
}

func downloadEmoji(emojiDir string) {
	htmlRes, err := http.Get(fmt.Sprintf("%s%s", lineEmoji, *emojiID))
	if err != nil {
		log.Fatal(err)
	}

	if htmlRes.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", htmlRes.StatusCode, htmlRes.Status)
	}

	defer htmlRes.Body.Close()
	doc, err := goquery.NewDocumentFromReader(htmlRes.Body)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find(".mdCMN09ImgList .mdCMN09ImgListWarp span.mdCMN09Image").Each(func(i int, s *goquery.Selection) {
		style, ok := s.Attr("style")
		if ok {
			// "background-image:url(https://stickershop.line-scdn.net/sticonshop/v1/sticon/5e4f906cd8824d19066dfc58/iPhone/007.png);"
			url := style[21 : len(style)-2]

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

			filename := strings.Split(style[len(style)-9:len(style)-2], ".")
			f, err := strconv.Atoi(filename[0])
			if err != nil {
				log.Println(err)
				return
			}
			resFilename := fmt.Sprintf("%d.%s", f, filename[1])
			p := filepath.Join(emojiDir, resFilename)
			err = ioutil.WriteFile(p, body, 0744)
			if err != nil {
				log.Println(err)
			}
		}
	})
}

func walkDir(path string) ([]string, error) {
	res := []string{}

	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for _, f := range files {
		p := filepath.Join(path, f.Name())
		res = append(res, p)
	}

	return res, nil
}

func emojiExist(name string, emojiList *listEmojiRes) bool {
	for k := range emojiList.Emoji {
		if name == k {
			return true
		}
	}

	return false
}

func upload(client *http.Client, buf *bytes.Buffer, contentType string) error {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/emoji.add", slackAPI), buf)
	req.Header.Set("Content-Type", contentType)
	if err != nil {
		log.Println(err)
		return err
	}

	for {
		resp, err := client.Do(req)
		if err != nil {
			log.Println(err)
			return err
		}
		defer resp.Body.Close()

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return err
		}

		r := &uploadEmojiRes{}
		err = json.Unmarshal(data, r)
		if err != nil {
			log.Println(err)
			return err
		}
		if !r.Ok {
			log.Println(r.Error)
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			log.Printf("too many requests, sleeping for %d seconds", throttle)
			time.Sleep(throttle * time.Second)
			continue
		}

		return nil
	}
}

func uploadEmoji(emojiDir string, curEmojiList *listEmojiRes) {
	files, err := walkDir(emojiDir)
	if err != nil {
		log.Fatal(err)
	}

	client := &http.Client{}

	for _, f := range files {
		n := strings.Split(filepath.Base(f), ".")[0]
		name := fmt.Sprintf("%s_%s", *prefix, n)
		if emojiExist(name, curEmojiList) {
			log.Printf("emoji code alread exist: %s", name)
			continue
		}

		file, err := os.Open(f)
		if err != nil {
			log.Println(err)
			continue
		}
		defer file.Close()

		bodyBuf := &bytes.Buffer{}
		writer := multipart.NewWriter(bodyBuf)

		part, err := writer.CreateFormFile("image", filepath.Base(f))
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = io.Copy(part, file)
		if err != nil {
			log.Println(err)
			continue
		}

		params := map[string]string{
			"mode":  "data",
			"name":  name,
			"token": *token,
		}
		for k, v := range params {
			writer.WriteField(k, v)
		}
		err = writer.Close()
		if err != nil {
			log.Println(err)
			continue
		}

		err = upload(client, bodyBuf, writer.FormDataContentType())
		if err != nil {
			continue
		}

		log.Printf("emoji added: %s, %s", name, filepath.Base(f))
	}
}

func main() {
	flag.Parse()
	if *token == "" || *emojiID == "" {
		panic("must provice token or id")
	}

	curEmojiList := getCurrentEmojis()

	emojiDir := prepareDir()
	downloadEmoji(emojiDir)
	uploadEmoji(emojiDir, curEmojiList)
	cleanDir(emojiDir)
}
