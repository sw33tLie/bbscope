package whttp

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"strings"
	"unicode/utf8"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/net/html"
)

type WHTTPHeader struct {
	Name  string
	Value string
}

type WHTTPReq struct {
	URL        string
	Method     string
	Body       string
	CustomHost string
	Headers    []WHTTPHeader
}

type WHTTPRes struct {
	StatusCode     int
	ResponseLength int
	HTTPTitle      string
	BodyString     string
	Headers        http.Header
}

var retryClient *retryablehttp.Client

func init() {
	retryClient = retryablehttp.NewClient()
	retryClient.RetryMax = 99999

	// Default timeout to 30 seconds
	retryClient.HTTPClient.Timeout = 30 * time.Second

	// Don't print debug messages
	retryClient.Logger = log.New(io.Discard, "", 0)
}

func GetDefaultClient() *retryablehttp.Client {
	return retryClient
}

func SendHTTPRequest(wReq *WHTTPReq, customClient *retryablehttp.Client) (wRes *WHTTPRes, err error) {
	client := customClient
	if client == nil {
		client = retryClient // Use the default client
	}

	var req *retryablehttp.Request
	if wReq.Body != "" {
		req, err = retryablehttp.NewRequest(wReq.Method, wReq.URL, strings.NewReader(wReq.Body))
	} else {
		req, err = retryablehttp.NewRequest(wReq.Method, wReq.URL, nil)
	}

	if err != nil {
		return nil, err
	}

	if wReq.CustomHost != "" {
		req.Host = wReq.CustomHost
	} else {
		if strings.HasSuffix(req.Host, ":80") {
			req.Host = strings.TrimSuffix(req.Host, ":80")
		} else if strings.HasSuffix(req.Host, ":443") {
			req.Host = strings.TrimSuffix(req.Host, ":443")
		}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:83.0) Gecko/20100101 Firefox/83.0 bbscope")
	req.Header.Set("Cache-Control", "no-transform")
	req.Header.Set("Connection", "close")
	req.Header.Set("Accept-Language", "en")

	if len(wReq.Headers) > 0 {
		for _, h := range wReq.Headers {
			req.Header.Add(h.Name, h.Value)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	wRes = &WHTTPRes{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	wRes.BodyString = string(bodyBytes)
	wRes.StatusCode = resp.StatusCode

	if title, ok := getHTMLTitle(wRes.BodyString); ok {
		wRes.HTTPTitle = strings.ToValidUTF8(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(title, "\n", ""), "\r", "")), "")
	}

	wRes.ResponseLength = utf8.RuneCountInString(wRes.BodyString)
	return wRes, nil
}

func SetupProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %v", err)
	}

	client := GetDefaultClient()
	client.HTTPClient.Transport = &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			},
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS11,
			MaxVersion:               tls.VersionTLS11,
		},
	}

	return nil
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
