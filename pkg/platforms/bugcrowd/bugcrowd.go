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
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
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

var (
	rateLimitRequestChan       chan rateLimitedRequest
	bugcrowdSessionCookieNames = []string{"_crowdcontrol_session_key"}
)

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

// Automated email + password login with Okta-based 2FA
func Login(email, password, otpFetchCommand, proxy string) (string, error) {
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

	// Step 1: GET identity.bugcrowd.com/login (redirects to bugcrowd.com/user/sign_in)
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

	// Extract csrf-token cookie from identity.bugcrowd.com (set by first request)
	identityUrl, _ := url.Parse("https://identity.bugcrowd.com")
	csrfTokenFromCookie := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(identityUrl) {
		if cookie.Name == "csrf-token" {
			csrfTokenFromCookie = cookie.Value
			break
		}
	}

	if csrfTokenFromCookie == "" {
		return "", errors.New("csrf-token not found in identity.bugcrowd.com cookies")
	}

	// Step 2: POST to identity.bugcrowd.com/login to get sessionToken
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

	// Extract sessionToken from redirect_to URL in response
	redirectTo := gjson.Get(firstLoginRes.BodyString, "redirect_to").String()
	if redirectTo == "" {
		return "", errors.New("redirect_to not found in login response")
	}

	// Parse the redirect_to URL to extract sessionToken
	redirectURL, err := url.Parse(redirectTo)
	if err != nil {
		return "", fmt.Errorf("failed to parse redirect_to URL: %v", err)
	}

	sessionToken := redirectURL.Query().Get("sessionToken")
	if sessionToken == "" {
		return "", errors.New("sessionToken not found in redirect_to URL")
	}

	// Step 3: GET bugcrowd.com/user/sign_in with sessionToken
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

	// Parse the sign-in HTML once so we can reuse it for CSRF/form extraction
	var (
		signInDoc *goquery.Document
		docErr    error
	)
	signInDoc, docErr = goquery.NewDocumentFromReader(strings.NewReader(signInRes.BodyString))
	if docErr != nil {
		utils.Log.Debug(fmt.Sprintf("failed to parse sign_in HTML: %v", docErr))
	}

	// Extract authenticity token from the sign_in page or cookies
	// IMPORTANT: We need the token from bugcrowd.com, not identity.bugcrowd.com
	// The token from identity.bugcrowd.com is not valid for bugcrowd.com endpoints
	authenticityToken := ""

	bugcrowdUrl, _ := url.Parse("https://bugcrowd.com")
	allCookies := retryClient.HTTPClient.Jar.Cookies(bugcrowdUrl)

	// First priority: Check bugcrowd.com cookies (set by sign_in page)
	for _, cookie := range allCookies {
		cookieName := strings.ToLower(cookie.Name)
		if cookieName == "csrf-token" || cookieName == "_csrf_token" || cookieName == "csrf_token" ||
			cookieName == "authenticity_token" || cookieName == "_authenticity_token" {
			authenticityToken = cookie.Value
			break
		}
	}

	// Second priority: Check response headers from sign_in page
	if authenticityToken == "" {
		csrfHeader := signInRes.Headers.Get("X-CSRF-Token")
		if csrfHeader != "" {
			authenticityToken = csrfHeader
		}
	}

	// Third priority: Check Set-Cookie headers from sign_in response
	if authenticityToken == "" {
		setCookies := signInRes.Headers.Values("Set-Cookie")
		for _, setCookie := range setCookies {
			if strings.Contains(strings.ToLower(setCookie), "csrf") || strings.Contains(strings.ToLower(setCookie), "authenticity") {
				// Extract cookie value from Set-Cookie header
				parts := strings.Split(setCookie, ";")
				if len(parts) > 0 {
					keyValue := strings.Split(parts[0], "=")
					if len(keyValue) == 2 {
						cookieName := strings.ToLower(strings.TrimSpace(keyValue[0]))
						if cookieName == "csrf-token" || cookieName == "_csrf_token" || cookieName == "csrf_token" ||
							cookieName == "authenticity_token" || cookieName == "_authenticity_token" {
							authenticityToken = keyValue[1]
							break
						}
					}
				}
			}
		}
	}

	// Fourth priority: Fall back to identity.bugcrowd.com token if bugcrowd.com token not found
	// (This shouldn't normally work, but might be needed in some cases)
	if authenticityToken == "" && csrfTokenFromCookie != "" {
		authenticityToken = csrfTokenFromCookie
	}

	// If not in cookies or headers, try to extract from HTML
	if authenticityToken == "" && docErr == nil && signInDoc != nil {
		// Try meta tag with various name variations
		signInDoc.Find("meta").Each(func(i int, s *goquery.Selection) {
			name, _ := s.Attr("name")
			content, exists := s.Attr("content")
			if exists && (name == "csrf-token" || name == "csrf_token" || name == "authenticity-token" || name == "authenticity_token") {
				authenticityToken = content
			}
		})

		// Try input fields
		if authenticityToken == "" {
			signInDoc.Find("input").Each(func(i int, s *goquery.Selection) {
				inputName, _ := s.Attr("name")
				if inputName == "authenticity_token" || inputName == "csrf_token" || inputName == "csrf-token" {
					if value, exists := s.Attr("value"); exists {
						authenticityToken = value
					}
				}
			})
		}

		// Try data attributes
		if authenticityToken == "" {
			signInDoc.Find("[data-csrf-token], [data-csrf_token], [data-authenticity-token]").Each(func(i int, s *goquery.Selection) {
				if token, exists := s.Attr("data-csrf-token"); exists && token != "" {
					authenticityToken = token
				} else if token, exists := s.Attr("data-csrf_token"); exists && token != "" {
					authenticityToken = token
				} else if token, exists := s.Attr("data-authenticity-token"); exists && token != "" {
					authenticityToken = token
				}
			})
		}

		// Try to find it in script tags
		if authenticityToken == "" {
			signInDoc.Find("script").Each(func(i int, s *goquery.Selection) {
				scriptContent := s.Text()
				// Try various patterns
				patterns := []string{
					`csrf[_-]?token["\s:=]+["']([^"']+)["']`,
					`authenticity[_-]?token["\s:=]+["']([^"']+)["']`,
					`"csrfToken"\s*:\s*"([^"]+)"`,
					`'csrfToken'\s*:\s*'([^']+)'`,
					`csrf[_-]?token\s*=\s*["']([^"']+)["']`,
				}
				for _, pattern := range patterns {
					re := regexp.MustCompile(pattern)
					matches := re.FindStringSubmatch(scriptContent)
					if len(matches) > 1 && matches[1] != "" {
						authenticityToken = matches[1]
						break
					}
				}
			})
		}
	}

	// Try regex patterns on the raw HTML (case-insensitive)
	if authenticityToken == "" {
		patterns := []string{
			`<meta\s+name=["']csrf[_-]?token["']\s+content=["']([^"']+)["']`,
			`<meta\s+content=["']([^"']+)["']\s+name=["']csrf[_-]?token["']`,
			`<input[^>]*name=["']authenticity[_-]?token["'][^>]*value=["']([^"']+)["']`,
			`<input[^>]*value=["']([^"']+)["'][^>]*name=["']authenticity[_-]?token["']`,
			`csrf[_-]?token["\s:=]+["']([^"']+)["']`,
			`"csrfToken"\s*:\s*"([^"]+)"`,
		}
		for _, pattern := range patterns {
			re := regexp.MustCompile("(?i)" + pattern)
			matches := re.FindStringSubmatch(signInRes.BodyString)
			if len(matches) > 1 && matches[1] != "" {
				authenticityToken = matches[1]
				break
			}
		}
	}

	// If still not found, this is a critical error
	if authenticityToken == "" {
		// Log a sample of the HTML to help debug
		htmlSample := signInRes.BodyString
		if len(htmlSample) > 500 {
			htmlSample = htmlSample[:500]
		}
		return "", fmt.Errorf("authenticity token not found in sign_in page. HTML sample: %s", htmlSample)
	}

	// Step 4: POST bugcrowd.com/user/auth/hacker with sessionToken
	// This redirects to Okta authorize endpoint
	// Note: authenticityToken is required, we've already validated it exists above
	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Content-Type", Value: "application/x-www-form-urlencoded"},
		{Name: "Origin", Value: "https://bugcrowd.com"},
		{Name: "Referer", Value: signInURL},
		{Name: "X-Csrf-Token", Value: authenticityToken}, // Match exact case from legitimate flow
	}

	formValues := url.Values{}
	authHackerURL := ""

	if signInDoc != nil && docErr == nil {
		formSel := signInDoc.Find("form#initiate_oidc_form")
		if formSel.Length() == 0 {
			formSel = signInDoc.Find("form[action*='/user/auth/hacker']").First()
		}

		if formSel.Length() > 0 {
			if action, exists := formSel.Attr("action"); exists {
				authHackerURL = action
			}

			formSel.Find("input").Each(func(i int, s *goquery.Selection) {
				if name, ok := s.Attr("name"); ok && name != "" {
					if value, exists := s.Attr("value"); exists {
						formValues.Set(name, value)
					}
				}
			})
		}
	}

	if formValues.Get("authenticity_token") == "" && authenticityToken != "" {
		formValues.Set("authenticity_token", authenticityToken)
	}
	if formValues.Get("utf8") == "" {
		formValues.Set("utf8", "✓")
	}

	if authHackerURL == "" {
		authHackerURL = fmt.Sprintf("https://bugcrowd.com/user/auth/hacker?acr_values=phr&login_hint=%s&origin=https%%3A%%2F%%2Fbugcrowd.com%%2Fdashboard&sessionToken=%s",
			url.QueryEscape(email), url.QueryEscape(sessionToken))
	}

	if strings.HasPrefix(authHackerURL, "/") {
		authHackerURL = "https://bugcrowd.com" + authHackerURL
	}

	body := formValues.Encode()

	authHackerRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "POST",
			URL:     authHackerURL,
			Headers: headers,
			Body:    body,
		}, retryClient)

	if err != nil {
		return "", err
	}

	if authHackerRes.StatusCode == 403 || authHackerRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	// Check for error response (400, 500, etc.)
	if authHackerRes.StatusCode >= 400 {
		errorPreview := authHackerRes.BodyString
		if len(errorPreview) > 1000 {
			errorPreview = errorPreview[:1000]
		}
		return "", fmt.Errorf("auth/hacker endpoint returned error %d: %s", authHackerRes.StatusCode, errorPreview)
	}

	// Check for error response in body
	if strings.Contains(authHackerRes.BodyString, "auth/failure") || strings.Contains(authHackerRes.BodyString, "InvalidAuthenticityToken") {
		return "", fmt.Errorf("authentication failed: %s", authHackerRes.BodyString)
	}

	// Extract the authorize URL from the redirect Location header
	// The POST to /user/auth/hacker should return a 302 redirect to the Okta authorize URL
	// with all the correct parameters (PKCE, nonce, etc.) that Bugcrowd generates
	authorizeURL := ""
	oktaStateTokenFromPage := ""
	oktaAuthorizeRes := authHackerRes

	if authHackerRes.StatusCode >= 300 && authHackerRes.StatusCode < 400 {
		// Extract Location header from redirect
		authorizeURL = authHackerRes.Headers.Get("Location")
		if authorizeURL == "" {
			return "", errors.New("redirect Location header not found in auth/hacker response")
		}

		// Make sure it's an absolute URL
		if !strings.HasPrefix(authorizeURL, "http") {
			baseURL, _ := url.Parse("https://bugcrowd.com")
			resolvedURL := baseURL.ResolveReference(&url.URL{Path: authorizeURL})
			authorizeURL = resolvedURL.String()
		}

		// Follow the redirect to Okta authorize endpoint
		// This will use cookies set during previous steps
		oktaAuthorizeRes, err = rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    authorizeURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Referer", Value: "https://bugcrowd.com/"},
					{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
					{Name: "Accept-Language", Value: "en-GB,en;q=0.9"},
					{Name: "Sec-Fetch-Site", Value: "same-site"},
					{Name: "Sec-Fetch-Mode", Value: "navigate"},
					{Name: "Sec-Fetch-Dest", Value: "document"},
				},
			}, retryClient)
		if err != nil {
			return "", err
		}
		oktaStateTokenFromPage = extractOktaStateToken(oktaAuthorizeRes.BodyString)
	} else if strings.Contains(authHackerRes.BodyString, "okta-sign-in") ||
		strings.Contains(authHackerRes.BodyString, "okta-signin-widget") {
		// Already at Okta page, try to extract the authorize URL from the page
		re := regexp.MustCompile(`https://login\.hackers\.bugcrowd\.com/oauth2/default/v1/authorize[^"'\s]+`)
		matches := re.FindString(authHackerRes.BodyString)
		if matches != "" {
			authorizeURL = matches
		}

		if authorizeURL == "" {
			redirectRe := regexp.MustCompile(`"redirectUri":"([^"]+)"`)
			redirectMatches := redirectRe.FindStringSubmatch(authHackerRes.BodyString)
			if len(redirectMatches) > 1 {
				candidate := decodeOktaEscapes(redirectMatches[1])
				if candidate != "" {
					authorizeURL = candidate
				}
			}
		}

		oktaStateTokenFromPage = extractOktaStateToken(authHackerRes.BodyString)
	} else {
		return "", fmt.Errorf("unexpected response from auth/hacker: status %d, expected redirect", authHackerRes.StatusCode)
	}

	if authorizeURL == "" {
		return "", errors.New("authorize URL not found in auth/hacker response")
	}

	stateToken := ""
	stateHandle := ""

	if oktaAuthorizeRes.StatusCode == 403 || oktaAuthorizeRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}

	// Check for error responses from Okta (only check status codes, not HTML content)
	if oktaAuthorizeRes.StatusCode >= 400 {
		// Try to extract JSON error message if it's a JSON response
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

	// Check for specific error messages in HTML (only if status is not 200)
	if oktaAuthorizeRes.StatusCode != 200 {
		if strings.Contains(oktaAuthorizeRes.BodyString, "unexpected internal error") ||
			strings.Contains(oktaAuthorizeRes.BodyString, "Something went wrong") {
			errorPreview := oktaAuthorizeRes.BodyString
			if len(errorPreview) > 500 {
				errorPreview = errorPreview[:500]
			}
			return "", fmt.Errorf("okta returned an error: %s", errorPreview)
		}
	}

	// Step 4: POST to Okta IDX introspect endpoint
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

	// Step 6: Run OTP generation command
	cmd := exec.Command("sh", "-c", otpFetchCommand)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute 2FA command: %v", err)
	}

	otpCode := strings.TrimSpace(string(output))
	if otpCode == "" {
		return "", fmt.Errorf("2FA code is empty")
	}

	// Step 7: POST OTP to Okta IDX challenge/answer endpoint
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

	// Check if OTP verification failed
	if gjson.Get(challengeRes.BodyString, "errorSummary").String() != "" {
		errorSummary := gjson.Get(challengeRes.BodyString, "errorSummary").String()
		return "", fmt.Errorf("OTP verification failed: %s", errorSummary)
	}

	// Extract new stateToken or sessionToken from challenge response
	updateOktaState(&stateToken, &stateHandle, challengeRes.BodyString)
	newStateToken := gjson.Get(challengeRes.BodyString, "sessionToken").String()
	if newStateToken == "" {
		if stateHandle != "" {
			newStateToken = stateHandle
		} else {
			newStateToken = stateToken
		}
	}

	// Step 9: GET Okta token redirect
	tokenRedirectURL := fmt.Sprintf("https://login.hackers.bugcrowd.com/login/token/redirect?stateToken=%s", url.QueryEscape(newStateToken))
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

	// Extract callback URL from redirect
	callbackURL := ""
	if tokenRedirectRes.StatusCode >= 300 && tokenRedirectRes.StatusCode < 400 {
		callbackURL = tokenRedirectRes.Headers.Get("Location")
	} else {
		// Try to extract from HTML
		re := regexp.MustCompile(`https://bugcrowd\.com/user/auth/hacker/callback[^"'\s]+`)
		matches := re.FindString(tokenRedirectRes.BodyString)
		if matches != "" {
			callbackURL = matches
		}
	}

	if callbackURL != "" {
		// Step 10: GET callback URL to complete OAuth flow
		callbackRes, err := rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    callbackURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Referer", Value: "https://login.hackers.bugcrowd.com/"},
				},
			}, retryClient)

		if err != nil {
			return "", err
		}

		if callbackRes.StatusCode == 403 || callbackRes.StatusCode == 406 {
			return "", errors.New(WAF_BANNED_ERROR)
		}

		// Step 11: Follow final redirect to dashboard
		dashboardURL := ""
		if callbackRes.StatusCode >= 300 && callbackRes.StatusCode < 400 {
			dashboardURL = callbackRes.Headers.Get("Location")
		} else {
			// Try to extract from HTML
			re := regexp.MustCompile(`https://bugcrowd\.com/dashboard[^"'\s]*`)
			matches := re.FindString(callbackRes.BodyString)
			if matches != "" {
				dashboardURL = matches
			}
		}

		if dashboardURL != "" {
			dashboardRes, err := rateLimitedSendHTTPRequest(
				&whttp.WHTTPReq{
					Method: "GET",
					URL:    dashboardURL,
					Headers: []whttp.WHTTPHeader{
						{Name: "User-Agent", Value: USER_AGENT},
						{Name: "Referer", Value: callbackURL},
					},
				}, retryClient)

			if err == nil && dashboardRes.StatusCode != 403 && dashboardRes.StatusCode != 406 {
				if sessionValue, sessionName := findSessionCookie(retryClient.HTTPClient.Jar, "https://bugcrowd.com"); sessionValue != "" {
					return logSessionSuccess(sessionName, sessionValue)
				}
			}
		}
	} else {
	}

	if sessionValue, sessionName := findSessionCookie(retryClient.HTTPClient.Jar, "https://bugcrowd.com"); sessionValue != "" {
		return logSessionSuccess(sessionName, sessionValue)
	}

	// Also check identity.bugcrowd.com cookies
	if sessionValue, sessionName := findSessionCookie(retryClient.HTTPClient.Jar, "https://identity.bugcrowd.com"); sessionValue != "" {
		return logSessionSuccess(sessionName, sessionValue)
	}

	return "", errors.New("unable to obtain session cookie after completing login flow")
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
					{Name: "Cookie", Value: buildSessionCookieHeader(sessionToken)},
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

	pData.Url = "https://bugcrowd.com/" + strings.TrimPrefix(handle, "/")

	if isEngagement {
		getBriefVersionDocument, err := getEngagementBriefVersionDocument(handle, token)
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
				{Name: "Cookie", Value: buildSessionCookieHeader(token)},
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
				{Name: "Cookie", Value: buildSessionCookieHeader(token)},
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
				{Name: "Cookie", Value: buildSessionCookieHeader(token)},
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
				{Name: "Cookie", Value: buildSessionCookieHeader(token)},
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

	for i := 0; i < int(targetsCount); i++ {
		targetPath := fmt.Sprintf("targets.%d", i)
		name := strings.TrimSpace(gjson.Get(json, targetPath+".name").String())
		uri := strings.TrimSpace(gjson.Get(json, targetPath+".uri").String())
		category := gjson.Get(json, targetPath+".category").String()
		description := gjson.Get(json, targetPath+".description").String()

		fetchedCategories, err := GetCategories(categories)

		if err != nil {
			return err
		}

		if categories != "all" && category != fetchedCategories[0] {
			continue
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

func GetCategories(input string) ([]string, error) {
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
		return nil, errors.New("invalid category")
	}
	return selectedCategory, nil
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

// decodeOktaEscapes converts Okta's \xNN sequences into valid characters and
// returns the unescaped redirect URI string.
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

func buildSessionCookieHeader(token string) string {
	return fmt.Sprintf("_crowdcontrol_session_key=%s", token)
}

func findSessionCookie(jar http.CookieJar, rawURL string) (string, string) {
	if jar == nil {
		return "", ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	cookies := jar.Cookies(parsed)
	for _, name := range bugcrowdSessionCookieNames {
		for _, cookie := range cookies {
			if cookie.Name == name && cookie.Value != "" {
				return cookie.Value, name
			}
		}
	}
	return "", ""
}

func logSessionSuccess(cookieName, cookieValue string) (string, error) {
	utils.Log.Info("Login OK. Fetching programs, please wait...")
	if cookieName != "" {
		utils.Log.Debug(fmt.Sprintf("Using %s cookie for session", cookieName))
	}
	return cookieValue, nil
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
	return authenticatorType == "password" || authenticatorKey == "password"
}
