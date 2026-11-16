package parser

import (
	"NUMParser/client"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

func get(link string) (string, error) {
	var body string
	var err error
	link = strings.ReplaceAll(link, "\t", "%20")
	for i := 0; i < 4; i++ {
		if strings.Contains(link, "rutor.lib") {
			body, err = client.GetNic(link, "", "")
		} else {
			_, bodyS, errs := client.Get(link).End()
			body = bodyS
			if len(errs) > 0 {
				err = errs[0]
			}
		}
		if err == nil || err == client.Err404 {
			break
		}
		log.Println("Error get page, attempt:", i+1, link, err)
		time.Sleep(time.Duration(2*(i+1)) * time.Second)
	}
	return body, err
}

func getBuf(link, referer, cookie string) ([]byte, error) {
	var body []byte
	var err error
	for i := 0; i < 4; i++ {
		_, bodyS, errs := client.GetParam(link, referer, cookie).EndBytes()
		body = bodyS
		if len(errs) > 0 {
			err = errs[0]
		}
		if err == nil {
			break
		}
		log.Println("Error get page, attempt:", i+1, link)
		time.Sleep(time.Duration(2*(i+1)) * time.Second)
	}
	return body, err
}

func node2Text(node *html.Node) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(strings.Replace((&goquery.Selection{Nodes: []*html.Node{node}}).Text(), "\u00A0", " ", -1))
}

func replaceBadName(name string) string {
	name = strings.ReplaceAll(name, "Ванда/Вижн ", "ВандаВижн ")
	name = strings.ReplaceAll(name, "Ё", "Е")
	name = strings.ReplaceAll(name, "ё", "е")
	name = strings.ReplaceAll(name, "щ", "ш")
	return name
}

func getHash(magnet string) string {
	lower := strings.ToLower(magnet)
	pos := strings.Index(lower, "btih:")
	if pos == -1 {
		return ""
	}
	hash := lower[pos+5:]
	if amp := strings.Index(hash, "&"); amp != -1 {
		hash = hash[:amp]
	}
	return hash
}
