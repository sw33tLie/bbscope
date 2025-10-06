package bugcrowd

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/otp"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	USER_AGENT               = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
	RATE_LIMIT_SLEEP_SECONDS = 5

	WAF_BANNED_ERROR = "you are temporarily WAF banned, change IP or wait a few hours"
)

// Rate-limiting types and global channel
type rateLimitedResult struct {
	res *whttp.WHTTPRes
	err error
}

type rateLimitedRequest struct {
	req        *whttp.WHTTPReq
	client     *retryablehttp.Client // Can be nil
	resultChan chan rateLimitedResult
}

var rateLimitRequestChan chan rateLimitedRequest

func init() {
	// Initialize the rate-limited request channel and start the worker
	rateLimitRequestChan = make(chan rateLimitedRequest)
	go rateLimitedRequestWorker()
}

func rateLimitedRequestWorker() {
	ticker := time.NewTicker(1 * time.Second) // one request per second (otherwise bugcrowd WAF bans us)
	defer ticker.Stop()
	for r := range rateLimitRequestChan {
		<-ticker.C // Wait for ticker; limits to one request per second
		res, err := whttp.SendHTTPRequest(r.req, r.client)
		r.resultChan <- rateLimitedResult{res: res, err: err}
	}
}

// rateLimitedSendHTTPRequest is a wrapper for whttp.SendHTTPRequest that enforces the 1-req/sec rate limit.
func rateLimitedSendHTTPRequest(req *whttp.WHTTPReq, client *retryablehttp.Client) (*whttp.WHTTPRes, error) {
	resultChan := make(chan rateLimitedResult, 1)
	rateLimitRequestChan <- rateLimitedRequest{
		req:        req,
		client:     client,
		resultChan: resultChan,
	}
	result := <-resultChan
	return result.res, result.err
}

// Automated email + password login. 2FA needs to be disabled
func Login(email, password, otpSecret, proxy string) (string, error) {
	// Create a cookie jar
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}

	// Create a retryablehttp client
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.New(io.Discard, "", 0)
	retryClient.RetryMax = 5 // Set your retry policy
	retryClient.HTTPClient.Jar = jar

	// Set proxy for custom client
	if proxy != "" {
		// Parse the proxy URL
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return "", fmt.Errorf("invalid proxy URL: %v", err)
		}

		// Apply proxy settings directly to this client
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
				MaxVersion:               tls.VersionTLS11,
			},
		}

		// Also update the global client for other requests
		whttp.SetupProxy(proxy)
	}

	firstRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://identity.bugcrowd.com/login?user_hint=researcher&returnTo=/dashboard",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
			},
		}, retryClient)

	if err != nil {
		return "", err
	}

	if firstRes.StatusCode == 403 || firstRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	identityUrl, _ := url.Parse("https://identity.bugcrowd.com")
	csrfToken := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "csrf-token" {
			csrfToken = cookie.Value
			break
		}
	}

	// Step 1: Initial login with username/password (without OTP)
	firstLoginRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/login",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&otp_code=&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)

	if err != nil {
		return "", err
	}

	if firstLoginRes.StatusCode == 403 || firstLoginRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	needsMfa := gjson.Get(firstLoginRes.BodyString, "needsMfa").Bool()
	if !needsMfa {
		return "", errors.New("unexpected response: MFA should be required")
	}

	otpCode, err := otp.GenerateTOTP(otpSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP: %v", err)
	}

	if otpCode == "" {
		return "", fmt.Errorf("2FA code is empty")
	}

	// Step 2: Submit OTP
	otpRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/auth/otp-challenge",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&otp_code=" + otpCode + "&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)

	if err != nil {
		return "", err
	}

	if otpRes.StatusCode == 403 || otpRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	// Check if OTP failed
	needsMfa = gjson.Get(otpRes.BodyString, "needsMfa").Bool()
	message := gjson.Get(otpRes.BodyString, "message").String()
	if needsMfa {
		return "", fmt.Errorf("2FA verification failed: %s", message)
	}

	redirectUrl := gjson.Get(otpRes.BodyString, "redirect_to").String()
	if redirectUrl == "" {
		return "", errors.New("no redirect URL found in response")
	}

	redirectRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    redirectUrl,
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
			},
		}, retryClient)

	if err != nil {
		return "", err
	}

	if redirectRes.StatusCode == 403 || redirectRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "_bugcrowd_session" {
			utils.Log.Info("Login OK. Fetching programs, please wait...")
			utils.Log.Debug("SESSION: ", cookie.Value)
			return cookie.Value, nil
		}
	}

	return "", errors.New("unable to obtain session cookie")
}

func GetProgramHandles(sessionToken string, engagementType string, pvtOnly bool) ([]string, error) {
	pageIndex := 1
	var totalCount int
	paths := []string{}
	fetchedPrograms := make(map[string]bool)
	allHandlersFoundCounter := 0

	listEndpointURL := "https://bugcrowd.com/engagements.json?category=" + engagementType + "&sort_by=promoted&sort_direction=desc&page="

	for {
		var res *whttp.WHTTPRes
		var err error

		res, err = rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    listEndpointURL + strconv.Itoa(pageIndex),
				Headers: []whttp.WHTTPHeader{
					{Name: "Cookie", Value: "_bugcrowd_session=" + sessionToken},
					{Name: "User-Agent", Value: USER_AGENT},
				},
			}, nil)

		if err != nil {
			return nil, err
		}

		if res.StatusCode == 403 || res.StatusCode == 406 {
			return nil, errors.New("you are temporarily WAF banned, change IP or wait a few hours")
		}

		// Assuming res.BodyString is the JSON string response
		result := gjson.Get(string(res.BodyString), "engagements")
		if totalCount == 0 {
			totalCount = int(gjson.Get(string(res.BodyString), "paginationMeta.totalCount").Int())
		}

		// If the engagements array is empty, it means there are no more programs to fetch on subsequent pages.
		if len(result.Array()) == 0 {
			break
		}

		// Iterating over each element in the programs array
		result.ForEach(func(key, value gjson.Result) bool {
			programURL := value.Get("briefUrl").String()
			accessStatus := value.Get("accessStatus").String()

			// Maintain a counter of unique program URLs found
			if !fetchedPrograms[programURL] {
				allHandlersFoundCounter++
				fetchedPrograms[programURL] = true

				if !pvtOnly || (pvtOnly && accessStatus != "open") {
					paths = append(paths, programURL)
				}
			}

			// Return true to continue iterating
			return true
		})

		// Print the number of programs fetched so far
		// utils.Log.Info("Fetched programs: ", len(paths), " | Total unique programs found: ", allHandlersFoundCounter)

		pageIndex++

		// Check if we have fetched all programs using allHandlersFoundCounter
		if allHandlersFoundCounter >= totalCount {
			break
		}
	}

	return paths, nil
}

func GetProgramScope(handle string, categories string, token string) (pData scope.ProgramData, err error) {
	isEngagement := strings.HasPrefix(handle, "/engagements/")
	if isEngagement {
		handle = strings.TrimPrefix(handle, "/engagements/")
	}

	pData.Url = "https://bugcrowd.com/" + strings.TrimPrefix(handle, "/")

	if isEngagement {
		getBriefVersionDocument, err := getEngagementBriefVersionDocument("/engagements/"+handle, token)
		if err != nil {
			return pData, err
		}

		if getBriefVersionDocument != "" {
			err = extractScopeFromEngagement(getBriefVersionDocument, token, &pData)
			if err != nil {
				return pData, err
			}
		}
	} else {
		err = extractScopeFromTargetGroups(pData.Url, categories, token, &pData)
		if err != nil {
			return pData, err
		}
	}

	return pData, nil
}

func getEngagementBriefVersionDocument(handle string, token string) (string, error) {
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://bugcrowd.com" + handle,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return "", err
	}

	if res.StatusCode == 403 || res.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	// Likely from a knownHandle we passed that's actually gone now
	if res.StatusCode == 404 {
		return "", nil // it's not an error for which we wanna exit the program
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))
	if err != nil {
		log.Fatal(err)
		return "", err
	}

	div := doc.Find("div[data-react-class='ResearcherEngagementBrief']")

	// Get the value of the data-api-endpoints attribute
	apiEndpointsJSON, exists := div.Attr("data-api-endpoints")
	if !exists {
		// This will be triggered when using a non-2FA token and
		if strings.Contains(res.BodyString, "ResearcherEngagementCompliance") {
			utils.Log.Warn("Compliance required! Skipping: ", "https://bugcrowd.com"+handle)
		} else {
			utils.Log.Warn("data-api-endpoints attribute not found at https://bugcrowd.com"+handle, res.StatusCode)
		}
	}

	return gjson.Get(apiEndpointsJSON, "engagementBriefApi.getBriefVersionDocument").String() + ".json", nil
}

func extractScopeFromEngagement(getBriefVersionDocument string, token string, pData *scope.ProgramData) (err error) {
	if getBriefVersionDocument == ".json" {
		utils.Log.Warn("Compliance required! Empty Extraction URL (Skipping)...")
		pData.InScope = append(pData.InScope, scope.ScopeElement{
			Target:      "2FA_REQUIRED",
			Description: "Two-Factor Authentication is required to access this program.",
		})
		return nil
	}
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://bugcrowd.com" + getBriefVersionDocument,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if res.StatusCode == 403 || res.StatusCode == 406 {
		return errors.New(WAF_BANNED_ERROR)
	}

	// Extract the "scope" array from the JSON
	scopeArray := gjson.Get(res.BodyString, "data.scope")

	// Iterate over each element of the "scope" array
	scopeArray.ForEach(func(key, value gjson.Result) bool {
		// Check if the scope element is in-scope
		inScope := value.Get("inScope").Bool()

		// Extract the "targets" array for the current scope element
		targetsArray := value.Get("targets")

		// Iterate over each object in the "targets" array
		targetsArray.ForEach(func(objectKey, objectValue gjson.Result) bool {
			// Extract the "name", "uri", "category", and "description" fields for each object
			name := objectValue.Get("name").String()
			uri := objectValue.Get("uri").String()
			category := objectValue.Get("category").String()
			description := objectValue.Get("description").String()

			if uri == "" {
				uri = name
			}

			if inScope {
				pData.InScope = append(pData.InScope, scope.ScopeElement{Target: uri, Description: description, Category: category})
			} else {
				pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{Target: uri, Description: description, Category: category})
			}

			return true
		})

		return true
	})

	return nil
}

func extractScopeFromTargetGroups(url string, categories string, token string, pData *scope.ProgramData) error {
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    url + "/target_groups",
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if res.StatusCode == 403 || res.StatusCode == 406 {
		return errors.New(WAF_BANNED_ERROR)
	}

	// Likely from a knownHandle we passed that's actually gone now
	if res.StatusCode == 404 {
		return nil // it's not an error for which we wanna exit the program
	}

	noScopeTable := true
	for i, scopeTableURL := range gjson.Get(string(res.BodyString), "groups.#.targets_url").Array() {
		inScope := gjson.Get(string(res.BodyString), fmt.Sprintf("groups.%d.in_scope", i)).Bool()
		err = extractScopeFromTargetTable(scopeTableURL.String(), categories, token, pData, inScope)
		if err != nil {
			return err
		}
		noScopeTable = false
	}

	if noScopeTable {
		pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE", Description: "", Category: ""})
	}

	return nil
}

func extractScopeFromTargetTable(scopeTableURL string, categories string, token string, pData *scope.ProgramData, inScope bool) error {
	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://bugcrowd.com" + scopeTableURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Cookie", Value: "_bugcrowd_session=" + token},
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return err
	}

	if res.StatusCode == 403 || res.StatusCode == 406 {
		return errors.New(WAF_BANNED_ERROR)
	}

	json := string(res.BodyString)
	targetsCount := gjson.Get(json, "targets.#").Int()

	// Get the list of categories to filter by.
	// If nil, we'll include all categories.
	selectedCategories := scope.GetAllStringsForCategories(categories)

	for i := 0; i < int(targetsCount); i++ {
		targetPath := fmt.Sprintf("targets.%d", i)
		name := strings.TrimSpace(gjson.Get(json, targetPath+".name").String())
		uri := strings.TrimSpace(gjson.Get(json, targetPath+".uri").String())
		category := gjson.Get(json, targetPath+".category").String()
		description := gjson.Get(json, targetPath+".description").String()

		// If selectedCategories is not nil (i.e., not "all"), then we filter.
		if selectedCategories != nil {
			catMatches := false
			for _, selectedCat := range selectedCategories {
				if category == selectedCat {
					catMatches = true
					break
				}
			}
			// If no match was found, skip this target.
			if !catMatches {
				continue
			}
		}

		if uri == "" {
			uri = name
		}

		scopeElement := scope.ScopeElement{
			Target:      uri,
			Description: description,
			Category:    category,
		}

		if inScope {
			pData.InScope = append(pData.InScope, scopeElement)
		} else {
			pData.OutOfScope = append(pData.OutOfScope, scopeElement)
		}
	}

	return nil
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, concurrency int, delimiterCharacter string, includeOOS, printRealTime bool, knownHandles []string) (programs []scope.ProgramData, err error) {
	programHandles, err := GetProgramHandles(token, "bug_bounty", pvtOnly)

	if err != nil {
		return nil, err
	}

	if !bbpOnly {
		vdpHandles, err := GetProgramHandles(token, "vdp", pvtOnly)
		if err != nil {
			return nil, err
		}
		programHandles = append(programHandles, vdpHandles...)
	}

	// Create a map to track existing handles
	existingHandles := make(map[string]bool)
	for _, handle := range programHandles {
		existingHandles[handle] = true
	}

	// Append unique handles from knownHandles to programHandles
	for _, handle := range knownHandles {
		if !existingHandles[handle] {
			programHandles = append(programHandles, handle)
			existingHandles[handle] = true
		}
	}

	utils.Log.Info("Fetching ", strconv.Itoa(len(programHandles)), " programs...")

	var mutex sync.Mutex
	handles := make(chan string, concurrency)
	errChan := make(chan error, 1)
	processGroup := new(sync.WaitGroup)

	for i := 0; i < concurrency; i++ {
		processGroup.Add(1)
		go func() {
			defer processGroup.Done()
			for handle := range handles {
				pScope, err := GetProgramScope(handle, categories, token)

				if err != nil {
					select {
					case errChan <- fmt.Errorf("error processing handle %s: %v", handle, err):
					default:
					}
					return
				}

				if len(pScope.InScope) == 0 {
					continue
				}

				mutex.Lock()
				programs = append(programs, pScope)
				mutex.Unlock()

				if printRealTime {
					scope.PrintProgramScope(pScope, outputFlags, delimiterCharacter, includeOOS)
				}
			}
		}()
	}

	go func() {
		for _, handle := range programHandles {
			handles <- handle
		}
		close(handles)
	}()

	go func() {
		processGroup.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return programs, err // Return partial results and the error
	}

	return programs, nil
}
