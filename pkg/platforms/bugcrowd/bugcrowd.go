package bugcrowd

import (
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	USER_AGENT               = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
	RATE_LIMIT_SLEEP_SECONDS = 5
)

// Automated email + password login. 2FA needs to be disabled
func Login(email, password, proxy string) string {
	cookies := make(map[string]string)

	var loginChallenge string

	// Create a cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		utils.Log.Fatal(err)
	}

	// Create a retryablehttp client
	retryClient := retryablehttp.NewClient()

	retryClient.Logger = log.New(io.Discard, "", 0)

	retryClient.RetryMax = 5 // Set your retry policy

	// Set the standard client's cookie jar
	retryClient.HTTPClient.Jar = jar

	// Set proxy for custom client

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			log.Fatal("Invalid Proxy String")
		}

		retryClient.HTTPClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				},
				PreferServerCipherSuites: true,
				MinVersion:               tls.VersionTLS11,
				MaxVersion:               tls.VersionTLS11},
		}
	}

	// Set the custom redirect policy on the underlying http.Client
	retryClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		utils.Log.Debug("Redirecting to: ", req.URL)
		if strings.Contains(req.URL.String(), "login_challenge") {
			loginChallenge = strings.Split(req.URL.String(), "=")[1]
		}
		return nil // return nil to follow the redirect
	}

	firstRes, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://identity.bugcrowd.com/login?user_hint=researcher&returnTo=/dashboard",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
			},
		}, retryClient)

	if err != nil {
		utils.Log.Fatal(err)
	}

	if firstRes.StatusCode == 403 {
		utils.Log.Fatal("Got 403 on first request. You may be WAF banned. Change IP or wait")
	}

	var allCookiesString string
	for _, cookie := range firstRes.Headers["Set-Cookie"] {
		split := strings.Split(cookie, ";")
		cookies[split[0]] = split[1]
		allCookiesString += split[0] + "=" + split[1] + "; "
	}

	identityUrl, _ := url.Parse("https://identity.bugcrowd.com")
	csrfToken := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "csrf-token" {
			csrfToken = cookie.Value
			break
		}
	}

	loginRes, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/login",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&login_challenge=" + loginChallenge + "&otp_code=&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)

	if err != nil {
		utils.Log.Fatal("Login request error: ", err)
	}

	if loginRes.StatusCode == 401 {
		utils.Log.Fatal("Login failed. Check your email and password. Make sure 2FA is off.")
	}

	_, err = whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    gjson.Get(loginRes.BodyString, "redirect_to").String(),
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
		}, retryClient)

	if err != nil {
		utils.Log.Fatal(err)
	}

	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "_bugcrowd_session" {
			utils.Log.Info("Login OK. Fetching programs, please wait...")
			utils.Log.Debug("SESSION: ", cookie.Value)
			return cookie.Value
		}
	}

	utils.Log.Fatal("Unknown Error")
	return ""
}

func GetProgramHandles(sessionToken string, engagementType string, pvtOnly bool) []string {
	pageIndex := 1

	listEndpointURL := "https://bugcrowd.com/engagements.json?category=" + engagementType + "&sort_by=promoted&sort_direction=desc&page="
	paths := []string{}

	for {
		var res *whttp.WHTTPRes
		var err error

		res, err = whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    listEndpointURL + strconv.Itoa(pageIndex),
				Headers: []whttp.WHTTPHeader{
					{Name: "Cookie", Value: "_bugcrowd_session=" + sessionToken},
					{Name: "User-Agent", Value: USER_AGENT},
				},
			}, nil)

		if err != nil {
			utils.Log.Fatal(err)
		}

		// Assuming res.BodyString is the JSON string response
		result := gjson.Get(string(res.BodyString), "engagements")

		// Bugcrowd's API sometimes tell us there are fewer pages than in reality, so we do it this way
		if len(result.Array()) == 0 {
			break
		}

		// Iterating over each element in the programs array
		result.ForEach(func(key, value gjson.Result) bool {
			programURL := value.Get("briefUrl").String()
			accessStatus := value.Get("accessStatus").String()

			if !pvtOnly || (pvtOnly && accessStatus != "open") {
				paths = append(paths, programURL)
			}

			// Return true to continue iterating
			return true
		})

		pageIndex++

	}

	return paths
}

func GetProgramScope(handle string, categories string, token string) (pData scope.ProgramData) {
	pData.Url = "https://bugcrowd.com" + handle

	var res, res2 *whttp.WHTTPRes
	var err error

	res, err = whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    pData.Url + "/target_groups",
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		utils.Log.Fatal(err)
	}

	// Times @arcwhite broke our code: #3 and counting :D
	noScopeTable := true
	for _, scopeTableURL := range gjson.Get(string(res.BodyString), "groups.#(in_scope==true)#.targets_url").Array() {

		// Send HTTP request for each table

		res2, err = whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    "https://bugcrowd.com" + scopeTableURL.String(),
				Headers: []whttp.WHTTPHeader{
					{Name: "Cookie", Value: "_bugcrowd_session=" + token},
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Accept", Value: "*/*"},
				},
			}, nil)

		if err != nil {
			utils.Log.Fatal(err)
		}

		chunkData := gjson.GetMany(string(res2.BodyString), "targets.#.name", "targets.#.category", "targets.#.description")
		for i := 0; i < len(chunkData[0].Array()); i++ {
			var currentTarget struct {
				line     string
				category string
			}
			currentTarget.line = strings.TrimSpace(chunkData[0].Array()[i].String())
			currentTarget.category = chunkData[1].Array()[i].String()

			if categories != "all" {
				catMatches := false
				if currentTarget.category == GetCategories(categories)[0] {
					catMatches = true
				}

				if catMatches {
					pData.InScope = append(pData.InScope, scope.ScopeElement{Target: currentTarget.line, Description: chunkData[2].Array()[i].String(), Category: currentTarget.category})
				}

			} else {
				pData.InScope = append(pData.InScope, scope.ScopeElement{Target: currentTarget.line, Description: chunkData[2].Array()[i].String(), Category: currentTarget.category})
			}

		}
		noScopeTable = false
	}

	if noScopeTable {
		pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE", Description: "", Category: ""})
	}

	return pData
}

func GetCategories(input string) []string {
	categories := map[string][]string{
		"url":      {"website"},
		"api":      {"api"},
		"mobile":   {"android", "ios"},
		"android":  {"android"},
		"apple":    {"ios"},
		"other":    {"other"},
		"hardware": {"hardware"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		utils.Log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, concurrency int, delimiterCharacter string, includeOOS, printRealTime bool) (programs []scope.ProgramData) {
	programHandles := GetProgramHandles(token, "bug_bounty", pvtOnly)

	if !bbpOnly {
		programHandles = append(programHandles, GetProgramHandles(token, "vdp", pvtOnly)...)
	}

	utils.Log.Info("Fetching ", strconv.Itoa(len(programHandles)), " programs...")

	var mutex sync.Mutex
	handles := make(chan string, concurrency)
	processGroup := new(sync.WaitGroup)

	for i := 0; i < concurrency; i++ {
		processGroup.Add(1)
		go func() {
			defer processGroup.Done()
			for handle := range handles {
				pScope := GetProgramScope(handle, categories, token)

				mutex.Lock()
				programs = append(programs, pScope)
				mutex.Unlock()

				if printRealTime {
					scope.PrintProgramScope(pScope, outputFlags, delimiterCharacter, includeOOS)
				}
			}
		}()
	}

	for _, handle := range programHandles {
		handles <- handle
	}

	close(handles)
	processGroup.Wait()
	return programs
}
