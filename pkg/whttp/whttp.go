package whttp

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

type WHTTPHeader struct {
	Name  string
	Value string
}

type WHTTPReq struct {
	URL        string
	Method     string
	CustomHost string
	Headers    []WHTTPHeader
}

type WHTTPRes struct {
	StatusCode     int
	ResponseLength int
	HTTPTitle      string
	BodyString     string
}

func SendHTTPRequest(wReq *WHTTPReq, client *http.Client) (wRes *WHTTPRes, err error) {
	var req *http.Request
	req, err = http.NewRequest(wReq.Method, wReq.URL, nil)

	if err != nil {
		return nil, err
	}

	// Set custom Host header
	if wReq.CustomHost != "" {
		req.Host = wReq.CustomHost
	} else {
		if strings.HasSuffix(req.Host, ":80") {
			req.Host = strings.TrimSuffix(req.Host, ":80")
		} else if strings.HasSuffix(req.Host, ":443") {
			req.Host = strings.TrimSuffix(req.Host, ":443")
		}
	}

	// Set common headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:83.0) Gecko/20100101 Firefox/83.0")
	req.Header.Set("Cache-Control", "no-transform")
	req.Header.Set("Connection", "close")
	req.Header.Set("Accept-Language", "en")

	// Set custom headers
	if len(wReq.Headers) > 0 {
		for _, h := range wReq.Headers {
			req.Header.Add(h.Name, h.Value)
		}
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	wRes = &WHTTPRes{}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	wRes.BodyString = string(bodyBytes)
	wRes.StatusCode = resp.StatusCode

	if title, ok := getHTMLTitle(wRes.BodyString); ok {
		wRes.HTTPTitle = strings.ToValidUTF8(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(title, "\n", ""), "\r", "")), "")
	}

	wRes.ResponseLength = utf8.RuneCountInString(wRes.BodyString)
	return wRes, nil
}

func isTitleElement(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == "title"
}

func traverse(n *html.Node) (string, bool) {
	if isTitleElement(n) {
		if n.FirstChild != nil {
			return n.FirstChild.Data, true
		}
		return "", true
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result, ok := traverse(c)
		if ok {
			return result, ok
		}
	}

	return "", false
}

func getHTMLTitle(requestBody string) (string, bool) {
	doc, err := html.Parse(strings.NewReader(requestBody))
	if err != nil {
		fmt.Println("Failed to parse HTML!")
		return "", true
	}

	return traverse(doc)
}
