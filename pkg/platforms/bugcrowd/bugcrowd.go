package bugcrowd

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
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

// Automated email + password login with Okta MFA flow.
func Login(email, password, otpSecret, proxy string) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}

	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.New(io.Discard, "", 0)
	retryClient.RetryMax = 0 // No retries for login flow — redirects have side effects
	retryClient.HTTPClient.Jar = jar

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return "", fmt.Errorf("invalid proxy URL: %v", err)
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
				MaxVersion:               tls.VersionTLS11,
			},
		}
		whttp.SetupProxy(proxy)
	}

	firstRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://identity.bugcrowd.com/login?user_hint=researcher&returnTo=https%3A%2F%2Fbugcrowd.com%2Fdashboard",
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

	identityURL, _ := url.Parse("https://identity.bugcrowd.com")
	csrfTokenFromCookie := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityURL) {
		if cookie.Name == "csrf-token" {
			csrfTokenFromCookie = cookie.Value
			break
		}
	}
	if csrfTokenFromCookie == "" {
		return "", errors.New("csrf-token not found in identity.bugcrowd.com cookies")
	}

	firstLoginRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://identity.bugcrowd.com/login",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "X-Csrf-Token", Value: csrfTokenFromCookie},
				{Name: "Content-Type", Value: "application/x-www-form-urlencoded; charset=UTF-8"},
				{Name: "Origin", Value: "https://identity.bugcrowd.com"},
				{Name: "Referer", Value: "https://identity.bugcrowd.com/login?user_hint=researcher&returnTo=https%3A%2F%2Fbugcrowd.com%2Fdashboard"},
			},
			Body: "username=" + url.QueryEscape(email) + "&password=" + url.QueryEscape(password) + "&otp_code=&backup_otp_code=&user_type=RESEARCHER&remember_me=true",
		}, retryClient)
	if err != nil {
		return "", err
	}
	if firstLoginRes.StatusCode == 403 || firstLoginRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	redirectTo := gjson.Get(firstLoginRes.BodyString, "redirect_to").String()
	if redirectTo == "" {
		return "", errors.New("redirect_to not found in login response")
	}

	// Follow the redirect_to URL. The redirect chain from /user/sign_in goes through
	// identity.bugcrowd.com and eventually lands on the Okta authorize page at
	// login.hackers.bugcrowd.com. The cookie jar follows all 302 redirects automatically.
	signInURL := redirectTo
	signInRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    signInURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Referer", Value: "https://identity.bugcrowd.com/login"},
			},
		}, retryClient)
	if err != nil {
		return "", err
	}
	if signInRes.StatusCode == 403 || signInRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	// After following redirects, we should be on the Okta sign-in page
	oktaAuthorizeRes := signInRes
	authorizeURL := signInURL // Used as Referer for subsequent Okta API calls

	// Try to find the actual authorize URL from the final response URL or page content
	re := regexp.MustCompile(`https://login\.hackers\.bugcrowd\.com/oauth2/[^"'\s]+`)
	if matches := re.FindString(signInRes.BodyString); matches != "" {
		authorizeURL = matches
	}

	oktaStateTokenFromPage := extractOktaStateToken(oktaAuthorizeRes.BodyString)

	if oktaAuthorizeRes.StatusCode >= 400 {
		if strings.Contains(oktaAuthorizeRes.Headers.Get("Content-Type"), "application/json") {
			errorMsg := gjson.Get(oktaAuthorizeRes.BodyString, "error").String()
			errorDescription := gjson.Get(oktaAuthorizeRes.BodyString, "error_description").String()
			if errorMsg != "" || errorDescription != "" {
				return "", fmt.Errorf("okta error: %s - %s", errorMsg, errorDescription)
			}
		}
		errorPreview := oktaAuthorizeRes.BodyString
		if len(errorPreview) > 500 {
			errorPreview = errorPreview[:500]
		}
		return "", fmt.Errorf("okta authorize endpoint returned error %d: %s", oktaAuthorizeRes.StatusCode, errorPreview)
	}
	if strings.Contains(oktaAuthorizeRes.BodyString, "unexpected internal error") ||
		strings.Contains(oktaAuthorizeRes.BodyString, "Something went wrong") {
		errorPreview := oktaAuthorizeRes.BodyString
		if len(errorPreview) > 500 {
			errorPreview = errorPreview[:500]
		}
		return "", fmt.Errorf("okta returned an error: %s", errorPreview)
	}

	stateToken := ""
	stateHandle := ""

	introspectReqBody := map[string]interface{}{}
	if oktaStateTokenFromPage != "" {
		introspectReqBody["stateToken"] = oktaStateTokenFromPage
	}
	introspectBodyJSON, _ := json.Marshal(introspectReqBody)

	introspectRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://login.hackers.bugcrowd.com/idp/idx/introspect",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Content-Type", Value: "application/json"},
				{Name: "Accept", Value: "application/json"},
				{Name: "X-Requested-With", Value: "XMLHttpRequest"},
				{Name: "Origin", Value: "https://login.hackers.bugcrowd.com"},
				{Name: "Referer", Value: authorizeURL},
			},
			Body: string(introspectBodyJSON),
		}, retryClient)
	if err != nil {
		return "", err
	}
	if introspectRes.StatusCode == 403 || introspectRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	updateOktaState(&stateToken, &stateHandle, introspectRes.BodyString)
	if stateHandle == "" && stateToken == "" {
		return "", errors.New("state token/handle not found in Okta introspect response")
	}

	requiresPasswordChallenge := authenticatorRequiresPassword(introspectRes.BodyString)

	if remediationExists(introspectRes.BodyString, "identify") {
		identifyBody := map[string]interface{}{
			"identifier": email,
		}
		addStateFields(identifyBody, stateHandle, stateToken)
		identifyBodyJSON, _ := json.Marshal(identifyBody)

		identifyRes, err := rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "POST",
				URL:    "https://login.hackers.bugcrowd.com/idp/idx/identify",
				Headers: []whttp.WHTTPHeader{
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Content-Type", Value: "application/json"},
					{Name: "Accept", Value: "application/json"},
					{Name: "X-Requested-With", Value: "XMLHttpRequest"},
					{Name: "Origin", Value: "https://login.hackers.bugcrowd.com"},
					{Name: "Referer", Value: authorizeURL},
				},
				Body: string(identifyBodyJSON),
			}, retryClient)
		if err != nil {
			return "", err
		}
		if identifyRes.StatusCode == 403 || identifyRes.StatusCode == 406 {
			return "", errors.New(WAF_BANNED_ERROR)
		}
		updateOktaState(&stateToken, &stateHandle, identifyRes.BodyString)
		if stateHandle == "" && stateToken == "" {
			return "", errors.New("state token/handle not found in Okta identify response")
		}
		requiresPasswordChallenge = authenticatorRequiresPassword(identifyRes.BodyString)
	}

	if requiresPasswordChallenge {
		passwordChallengeBody := map[string]interface{}{
			"credentials": map[string]interface{}{
				"passcode": password,
			},
		}
		addStateFields(passwordChallengeBody, stateHandle, stateToken)
		passwordChallengeBodyJSON, _ := json.Marshal(passwordChallengeBody)

		passwordChallengeRes, err := rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "POST",
				URL:    "https://login.hackers.bugcrowd.com/idp/idx/challenge/answer",
				Headers: []whttp.WHTTPHeader{
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Content-Type", Value: "application/json"},
					{Name: "Accept", Value: "application/json"},
					{Name: "X-Requested-With", Value: "XMLHttpRequest"},
					{Name: "Origin", Value: "https://login.hackers.bugcrowd.com"},
					{Name: "Referer", Value: authorizeURL},
				},
				Body: string(passwordChallengeBodyJSON),
			}, retryClient)
		if err != nil {
			return "", err
		}
		if passwordChallengeRes.StatusCode == 403 || passwordChallengeRes.StatusCode == 406 {
			return "", errors.New(WAF_BANNED_ERROR)
		}
		if gjson.Get(passwordChallengeRes.BodyString, "errorSummary").String() != "" {
			errorSummary := gjson.Get(passwordChallengeRes.BodyString, "errorSummary").String()
			return "", fmt.Errorf("password verification failed: %s", errorSummary)
		}
		updateOktaState(&stateToken, &stateHandle, passwordChallengeRes.BodyString)
		if stateHandle == "" && stateToken == "" {
			return "", errors.New("state token/handle not found in Okta password challenge response")
		}
	}

	otpCode, err := otp.GenerateTOTP(otpSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP: %v", err)
	}
	if otpCode == "" {
		return "", fmt.Errorf("2FA code is empty")
	}

	challengeAnswerBody := map[string]interface{}{
		"credentials": map[string]interface{}{
			"passcode": otpCode,
		},
	}
	addStateFields(challengeAnswerBody, stateHandle, stateToken)
	challengeAnswerBodyJSON, _ := json.Marshal(challengeAnswerBody)

	challengeRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    "https://login.hackers.bugcrowd.com/idp/idx/challenge/answer",
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Content-Type", Value: "application/json"},
				{Name: "Accept", Value: "application/json"},
				{Name: "X-Requested-With", Value: "XMLHttpRequest"},
				{Name: "Origin", Value: "https://login.hackers.bugcrowd.com"},
				{Name: "Referer", Value: authorizeURL},
			},
			Body: string(challengeAnswerBodyJSON),
		}, retryClient)
	if err != nil {
		return "", err
	}
	if challengeRes.StatusCode == 403 || challengeRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}
	if gjson.Get(challengeRes.BodyString, "errorSummary").String() != "" {
		errorSummary := gjson.Get(challengeRes.BodyString, "errorSummary").String()
		return "", fmt.Errorf("OTP verification failed: %s", errorSummary)
	}

	// The OTP challenge response contains a success.href with the token redirect URL.
	// This URL has the correct short stateToken (not the full stateHandle).
	tokenRedirectURL := gjson.Get(challengeRes.BodyString, "success.href").String()
	if tokenRedirectURL == "" {
		// Fallback: try to extract stateToken from the response
		updateOktaState(&stateToken, &stateHandle, challengeRes.BodyString)
		newStateToken := gjson.Get(challengeRes.BodyString, "sessionToken").String()
		if newStateToken == "" {
			if stateToken != "" {
				newStateToken = stateToken
			} else if stateHandle != "" {
				// Use only the part before ~c. as the actual stateToken
				if idx := strings.Index(stateHandle, "~"); idx != -1 {
					newStateToken = stateHandle[:idx]
				} else {
					newStateToken = stateHandle
				}
			}
		}
		tokenRedirectURL = fmt.Sprintf("https://login.hackers.bugcrowd.com/login/token/redirect?stateToken=%s", url.QueryEscape(newStateToken))
	}
	tokenRedirectRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    tokenRedirectURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Referer", Value: authorizeURL},
			},
		}, retryClient)
	if err != nil {
		return "", err
	}
	if tokenRedirectRes.StatusCode == 403 || tokenRedirectRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	if cookieHeader := buildCookieHeaderFromJar(retryClient.HTTPClient.Jar, "https://bugcrowd.com"); cookieHeader != "" {
		utils.Log.Info("Login OK. Fetching programs, please wait...")
		return cookieHeader, nil
	}

	return "", errors.New("unable to obtain session cookie after completing login flow")
}

// StartSessionKeepalive starts a background goroutine that periodically refreshes
// the Bugcrowd session by hitting /auth/session. Returns a stop function.
func StartSessionKeepalive(sessionToken string, interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				res, err := rateLimitedSendHTTPRequest(
					&whttp.WHTTPReq{
						Method: "GET",
						URL:    "https://bugcrowd.com/auth/session?update_activity=false",
						Headers: []whttp.WHTTPHeader{
							{Name: "User-Agent", Value: USER_AGENT},
							{Name: "Cookie", Value: buildSessionCookieHeader(sessionToken)},
							{Name: "Accept", Value: "*/*"},
						},
					}, nil)
				if err != nil {
					utils.Log.Debug("Session keepalive failed: ", err)
				} else {
					remaining := gjson.Get(res.BodyString, "secondsRemaining").Int()
					utils.Log.Debug(fmt.Sprintf("Session keepalive OK, %d seconds remaining", remaining))
				}
			}
		}
	}()
	return func() { close(stop) }
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

		headers := []whttp.WHTTPHeader{
			{Name: "User-Agent", Value: USER_AGENT},
		}
		if sessionToken != "" {
			headers = append(headers, whttp.WHTTPHeader{Name: "Cookie", Value: buildSessionCookieHeader(sessionToken)})
		}

		res, err = rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method:  "GET",
				URL:     listEndpointURL + strconv.Itoa(pageIndex),
				Headers: headers,
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

	if isEngagement {
		pData.Url = "https://bugcrowd.com/engagements/" + strings.TrimPrefix(handle, "/")
	} else {
		pData.Url = "https://bugcrowd.com/" + strings.TrimPrefix(handle, "/")
	}

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
	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Accept", Value: "*/*"},
	}
	if token != "" {
		headers = append(headers, whttp.WHTTPHeader{Name: "Cookie", Value: buildSessionCookieHeader(token)})
	}

	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "GET",
			URL:     "https://bugcrowd.com" + handle,
			Headers: headers,
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
	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Accept", Value: "*/*"},
	}
	if token != "" {
		headers = append(headers, whttp.WHTTPHeader{Name: "Cookie", Value: buildSessionCookieHeader(token)})
	}

	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "GET",
			URL:     "https://bugcrowd.com" + getBriefVersionDocument,
			Headers: headers,
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
	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Accept", Value: "*/*"},
	}
	if token != "" {
		headers = append(headers, whttp.WHTTPHeader{Name: "Cookie", Value: buildSessionCookieHeader(token)})
	}

	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "GET",
			URL:     url + "/target_groups",
			Headers: headers,
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
	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Accept", Value: "*/*"},
	}
	if token != "" {
		headers = append(headers, whttp.WHTTPHeader{Name: "Cookie", Value: buildSessionCookieHeader(token)})
	}

	res, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "GET",
			URL:     "https://bugcrowd.com" + scopeTableURL,
			Headers: headers,
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

// buildSessionCookieHeader returns the token as a Cookie header value.
// After Login(), the token is already a full cookie string (all cookies from the jar).
// When passed via --token flag with a raw value, it wraps it as _bugcrowd_session=<value>.
func buildSessionCookieHeader(token string) string {
	if strings.Contains(token, "=") {
		return token
	}
	return "_bugcrowd_session=" + token
}

// buildCookieHeaderFromJar extracts all cookies from the jar for the given URL
// and returns them as a Cookie header string, deduplicating by name.
func buildCookieHeaderFromJar(jar http.CookieJar, rawURL string) string {
	if jar == nil {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	cookies := jar.Cookies(parsed)
	if len(cookies) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if seen[cookie.Name] {
			continue
		}
		seen[cookie.Name] = true
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func decodeOktaEscapes(input string) string {
	if input == "" {
		return ""
	}

	hexRe := regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)
	withUnicode := hexRe.ReplaceAllString(input, `\\u00$1`)
	withUnicode = strings.ReplaceAll(withUnicode, `\\u`, `\u`)

	unquoted, err := strconv.Unquote(`"` + withUnicode + `"`)
	if err != nil {
		return input
	}

	return unquoted
}

func extractOktaStateToken(html string) string {
	if html == "" {
		return ""
	}
	re := regexp.MustCompile(`"stateToken":"([^"]+)"`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		return decodeOktaEscapes(match[1])
	}
	return ""
}

func updateOktaState(stateToken, stateHandle *string, body string) {
	if stateHandle == nil || stateToken == nil {
		return
	}

	if newHandle := gjson.Get(body, "stateHandle").String(); newHandle != "" {
		*stateHandle = decodeOktaEscapes(newHandle)
	}
	if newToken := gjson.Get(body, "stateToken").String(); newToken != "" {
		*stateToken = decodeOktaEscapes(newToken)
	}
	if *stateToken == "" {
		if token := gjson.Get(body, "token").String(); token != "" {
			*stateToken = decodeOktaEscapes(token)
		}
	}
}

func addStateFields(body map[string]interface{}, stateHandle, stateToken string) {
	if body == nil {
		return
	}
	if stateHandle != "" {
		body["stateHandle"] = stateHandle
	}
	if stateToken != "" {
		body["stateToken"] = stateToken
	}
}

func remediationExists(body, name string) bool {
	found := false
	gjson.Get(body, "remediation.value").ForEach(func(_, value gjson.Result) bool {
		if strings.EqualFold(value.Get("name").String(), name) {
			found = true
			return false
		}
		return true
	})
	return found
}

func authenticatorRequiresPassword(body string) bool {
	authenticatorType := strings.ToLower(gjson.Get(body, "currentAuthenticatorEnrollment.type").String())
	authenticatorKey := strings.ToLower(gjson.Get(body, "currentAuthenticatorEnrollment.key").String())
	if authenticatorType == "password" || authenticatorKey == "password" || authenticatorKey == "okta_password" {
		return true
	}
	// Also check if the challenge-authenticator remediation has a passcode field with label containing "Password"
	found := false
	gjson.Get(body, "remediation.value").ForEach(func(_, rem gjson.Result) bool {
		if strings.EqualFold(rem.Get("name").String(), "challenge-authenticator") {
			rem.Get("value").ForEach(func(_, field gjson.Result) bool {
				if field.Get("name").String() == "credentials" {
					field.Get("form.value").ForEach(func(_, formField gjson.Result) bool {
						label := strings.ToLower(formField.Get("label").String())
						if strings.Contains(label, "password") {
							found = true
							return false
						}
						return true
					})
				}
				return !found
			})
		}
		return !found
	})
	return found
}
