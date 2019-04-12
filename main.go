package main

import (
	"fmt"
	baseLog "log"
	"net/http"
	netUrl "net/url"
	"os"
	"regexp"
	"strings"

	"github.com/yanzay/tbot"
	"golang.org/x/net/html"
)

var bot *tbot.Server
var bc *tbot.Client
var re string
var log tbot.BasicLogger

func isBroken(link string) bool {

	log.Debugf("Checking %s", link)
	client := &http.Client{}

	req, err := http.NewRequest("HEAD", link, nil)

	if err != nil {
		log.Print(err)
		return false
	}

	req.Header.Add("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/73.0.3683.86 Safari/537.36")

	resp, err := client.Do(req)

	if err != nil {
		log.Printf("ERROR: %v", err)
		return true
	}

	log.Debugf("StatusCode %d", resp.StatusCode)

	return resp.StatusCode > 299 && resp.StatusCode < 600
}

func getHref(t html.Token) (ok bool, href string) {
	for _, a := range t.Attr {
		if a.Key == "href" {
			href = strings.TrimSpace(a.Val)
			ok = len(href) != 0
		}
	}
	return
}

func getBaseURL(url string) (bool, string) {

	u, err := netUrl.Parse(url)

	if err != nil {
		log.Error(err)
		return false, ""
	}

	log.Printf("Request URI: %s\n", u.RequestURI())

	// Check if url has no path
	if strings.Compare(u.RequestURI(), "/") == 0 {

		// Check if url ends with "/": if yes, return url without it
		if strings.Compare(url[len(url)-1:], "/") == 0 {
			return true, url[0 : len(url)-1]
		}

		return true, url
	}
	return true, (url[0:strings.Index(url, u.RequestURI())])
}

func getAllLinks(url string) (bool, []string) {

	log.Printf("Crawling: %s", url)

	resp, err := http.Get(url)

	if err != nil {
		log.Print(err)
		return false, []string{}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// log.Printf("StatusCode: %d", resp.StatusCode)
		return false, []string{}
	}

	urls := []string{}
	b := resp.Body
	defer b.Close()

	z := html.NewTokenizer(b)

	for tt := z.Next(); tt != html.ErrorToken; tt = z.Next() {
		if t := z.Token(); tt == html.StartTagToken && t.Data == "a" {
			if ok, foundURL := getHref(t); ok {
				urls = append(urls, foundURL)
			}
		}
	}
	log.Printf("Extracted %d links", len(urls))
	return true, urls
}

func getBrokenLinks(url string, base string, mapURLS map[string]bool) (bool, []string) {

	ok, allLinks := getAllLinks(url)

	if !ok {
		return true, []string{url}
	}

	var urls []string

	for _, foundURL := range allLinks {

		if strings.Index(foundURL, "#") == 0 || strings.Compare(foundURL, "/") == 0 {
			continue
		}

		if !regexp.MustCompile(re).MatchString(foundURL) {
			if strings.Index(foundURL, "/") == 0 {
				foundURL = base + foundURL
			} else {
				foundURL = base + "/" + foundURL
			}
		}

		if mapURLS[foundURL] || mapURLS[foundURL+"/"] || mapURLS[foundURL[0:len(foundURL)-1]] {
			continue
		}

		mapURLS[foundURL] = true

		if isBroken(foundURL) {
			log.Debugf("from: %s", url)
			urls = append(urls, foundURL)
			continue
		}

		if strings.Index(foundURL, base) == 0 {
			if ok, newURLs := getBrokenLinks(foundURL, base, mapURLS); ok && len(newURLs) != 0 {
				urls = append(urls, newURLs...)
			}
		}
	}

	return true, urls
}

func botRoutine(message *tbot.Message) {

	log.Printf("Received %s \n", message.Text)

	chatID := message.Chat.ID

	bc.SendChatAction(chatID, tbot.ActionTyping)

	if !regexp.MustCompile(re).MatchString(message.Text) {
		bc.SendMessage(chatID, "The URL must be like \"http(s)://example.com/\"")
		return
	}

	url := message.Text

	// This map will be used to keep track of already checked urls
	mapURLS := make(map[string]bool)
	mapURLS[url] = true

	ok, base := getBaseURL(url)

	if !ok {
		bc.SendMessage(chatID, "Internal error")
		return
	}

	log.Debugf("Base: %s", base)
	log.Debugf("---- START ----")

	ok, urls := getBrokenLinks(url, base, mapURLS)

	if !ok {
		bc.SendMessage(chatID, fmt.Sprintf("%s is ureachable!", url))
		return
	}

	log.Debugf("---- END ----")

	msg := fmt.Sprintf("%d broken links found\n", len(urls))

	for _, l := range urls {
		msg = fmt.Sprintf("%s- %s\n", msg, l)
	}

	bc.SendMessage(chatID, msg)
}

func main() {

	f, err := os.OpenFile("debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)

	if err != nil {
		baseLog.Fatalf("error opening file: %v", err)
	}

	defer f.Close()

	baseLog.SetOutput(f)

	log = tbot.BasicLogger{}

	re = `https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,6}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`

	token := os.Getenv("TELEGRAM_TOKEN")

	if len(token) == 0 {
		baseLog.Fatalf("ERROR: telegram token not set")
	}

	bot = tbot.New(token, tbot.WithLogger(log))

	bc = bot.Client()
	// whitelist := []string{"yanzay", "user2"}

	bot.HandleMessage(".*", botRoutine)

	log.Printf("starting bot..\n")

	baseLog.Fatal(bot.Start())
}
