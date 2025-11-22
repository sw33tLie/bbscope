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

var (
	rateLimitRequestChan chan rateLimitedRequest
)

const bugcrowdSessionCookieName = "_crowdcontrol_session_key"

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
	retryClient.RetryMax = 5
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

	redirectURL, err := url.Parse(redirectTo)
	if err != nil {
		return "", fmt.Errorf("failed to parse redirect_to URL: %v", err)
	}
	sessionToken := redirectURL.Query().Get("sessionToken")
	if sessionToken == "" {
		return "", errors.New("sessionToken not found in redirect_to URL")
	}

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

	var (
		signInDoc *goquery.Document
		docErr    error
	)
	signInDoc, docErr = goquery.NewDocumentFromReader(strings.NewReader(signInRes.BodyString))
	if docErr != nil {
		utils.Log.Debug(fmt.Sprintf("failed to parse sign_in HTML: %v", docErr))
	}

	// Parse HTML to find tokens and form values
	var (
		authenticityToken string
		csrfTokenHeader   string
		authHackerURL     string
		formValues        = url.Values{}
	)

	if signInDoc != nil && docErr == nil {
		// 1. Find authenticity_token from input (for form body)
		signInDoc.Find("input").Each(func(i int, s *goquery.Selection) {
			name, _ := s.Attr("name")
			value, _ := s.Attr("value")
			if name == "authenticity_token" {
				authenticityToken = value
			}
			if name != "" {
				formValues.Set(name, value)
			}
		})

		// 2. Find csrfToken from SplashScreen data (for X-Csrf-Token header)
		// <div data-react-class="SplashScreen" data-react-props="{&quot;csrfToken&quot;:&quot;...&quot; ...}">
		signInDoc.Find("div[data-react-class='SplashScreen']").Each(func(i int, s *goquery.Selection) {
			props, exists := s.Attr("data-react-props")
			if exists {
				csrfTokenHeader = gjson.Get(props, "csrfToken").String()
			}
		})

		// Fallback for header if SplashScreen not found: use cookie or input
		if csrfTokenHeader == "" {
			// Try cookie from identity.bugcrowd.com (preserved in jar?)
			// The jar logic was complex, let's stick to what the page provides or fallback to cookie jar lookup if strictly needed.
			// Current implementation behavior: prefer cookie for header.
			// Let's check cookies explicitly again if needed.
			bugcrowdURL, _ := url.Parse("https://bugcrowd.com")
			for _, cookie := range retryClient.HTTPClient.Jar.Cookies(bugcrowdURL) {
				if strings.EqualFold(cookie.Name, "csrf-token") {
					csrfTokenHeader = cookie.Value
					break
				}
			}
		}
		// If still empty, fallback to the input token (better than nothing)
		if csrfTokenHeader == "" {
			csrfTokenHeader = authenticityToken
		}
		// If still empty, fallback to the identity cookie captured earlier
		if csrfTokenHeader == "" && csrfTokenFromCookie != "" {
			csrfTokenHeader = csrfTokenFromCookie
		}

		// 3. Find action URL
		formSel := signInDoc.Find("form#initiate_oidc_form")
		if formSel.Length() == 0 {
			formSel = signInDoc.Find("form[action*='/user/auth/hacker']").First()
		}
		if formSel.Length() > 0 {
			if action, exists := formSel.Attr("action"); exists {
				authHackerURL = action
			}
		}
	}

	if authenticityToken == "" {
		return "", errors.New("authenticity_token not found in sign_in page")
	}

	// Ensure form values are set
	if formValues.Get("authenticity_token") == "" {
		formValues.Set("authenticity_token", authenticityToken)
	}
	if formValues.Get("utf8") == "" {
		formValues.Set("utf8", "✓")
	}

	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Content-Type", Value: "application/x-www-form-urlencoded"},
		{Name: "Origin", Value: "https://bugcrowd.com"},
		{Name: "Referer", Value: signInURL},
		{Name: "X-Csrf-Token", Value: csrfTokenHeader},
	}

	if authHackerURL == "" {
		authHackerURL = fmt.Sprintf("https://bugcrowd.com/user/auth/hacker?acr_values=phr&login_hint=%s&origin=https%%3A%%2F%%2Fbugcrowd.com%%2Fdashboard&sessionToken=%s",
			url.QueryEscape(email), url.QueryEscape(sessionToken))
	}
	if strings.HasPrefix(authHackerURL, "/") {
		authHackerURL = "https://bugcrowd.com" + authHackerURL
	}

	authHackerRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method:  "POST",
			URL:     authHackerURL,
			Headers: headers,
			Body:    formValues.Encode(),
		}, retryClient)
	if err != nil {
		return "", err
	}
	if authHackerRes.StatusCode == 403 || authHackerRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}
	if authHackerRes.StatusCode >= 400 {
		errorPreview := authHackerRes.BodyString
		if len(errorPreview) > 1000 {
			errorPreview = errorPreview[:1000]
		}
		return "", fmt.Errorf("auth/hacker endpoint returned error %d: %s", authHackerRes.StatusCode, errorPreview)
	}
	if strings.Contains(authHackerRes.BodyString, "auth/failure") || strings.Contains(authHackerRes.BodyString, "InvalidAuthenticityToken") {
		return "", fmt.Errorf("authentication failed: %s", authHackerRes.BodyString)
	}

	authorizeURL := ""
	oktaStateTokenFromPage := ""
	oktaAuthorizeRes := authHackerRes

	if authHackerRes.StatusCode >= 300 && authHackerRes.StatusCode < 400 {
		authorizeURL = authHackerRes.Headers.Get("Location")
		if authorizeURL == "" {
			return "", errors.New("redirect Location header not found in auth/hacker response")
		}
		if !strings.HasPrefix(authorizeURL, "http") {
			baseURL, _ := url.Parse("https://bugcrowd.com")
			resolvedURL := baseURL.ResolveReference(&url.URL{Path: authorizeURL})
			authorizeURL = resolvedURL.String()
		}
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
		if jsonData := extractOktaDataJSON(oktaAuthorizeRes.BodyString); jsonData != "" {
			oktaStateTokenFromPage = gjson.Get(jsonData, "signIn.stateToken").String()
			if oktaStateTokenFromPage == "" {
				oktaStateTokenFromPage = gjson.Get(jsonData, "stateToken").String()
			}
		}
	} else if strings.Contains(authHackerRes.BodyString, "okta-sign-in") ||
		strings.Contains(authHackerRes.BodyString, "okta-signin-widget") {

		if jsonData := extractOktaDataJSON(authHackerRes.BodyString); jsonData != "" {
			authorizeURL = gjson.Get(jsonData, "redirectUri").String()
			oktaStateTokenFromPage = gjson.Get(jsonData, "signIn.stateToken").String()
			if oktaStateTokenFromPage == "" {
				oktaStateTokenFromPage = gjson.Get(jsonData, "stateToken").String()
			}
		}
	} else {
		return "", fmt.Errorf("unexpected response from auth/hacker: status %d, expected redirect", authHackerRes.StatusCode)
	}

	if authorizeURL == "" {
		return "", errors.New("authorize URL not found in auth/hacker response")
	}

	if oktaAuthorizeRes.StatusCode == 403 || oktaAuthorizeRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}
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

	updateOktaState(&stateToken, &stateHandle, challengeRes.BodyString)
	newStateToken := gjson.Get(challengeRes.BodyString, "sessionToken").String()
	if newStateToken == "" {
		if stateHandle != "" {
			newStateToken = stateHandle
		} else {
			newStateToken = stateToken
		}
	}

	tokenRedirectURL := fmt.Sprintf("https://login.hackers.bugcrowd.com/login/token/redirect?stateToken=%s", url.QueryEscape(newStateToken))

	// This will redirect till we reach /dashboard
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

	// Follow redirects until we reach the final page (dashboard)
	// Note: retryablehttp follows redirects automatically, but we handle additional redirects manually if needed
	finalRes := tokenRedirectRes
	currentURL := tokenRedirectURL
	maxRedirects := 10
	redirectCount := 0

	for redirectCount < maxRedirects && (finalRes.StatusCode >= 300 && finalRes.StatusCode < 400) {
		redirectURL := finalRes.Headers.Get("Location")
		if redirectURL == "" {
			break
		}
		if !strings.HasPrefix(redirectURL, "http") {
			baseURL, _ := url.Parse(currentURL)
			resolvedURL := baseURL.ResolveReference(&url.URL{Path: redirectURL})
			redirectURL = resolvedURL.String()
		}
		prevURL := currentURL
		currentURL = redirectURL

		finalRes, err = rateLimitedSendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    redirectURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "User-Agent", Value: USER_AGENT},
					{Name: "Referer", Value: prevURL},
				},
			}, retryClient)
		if err != nil {
			return "", err
		}
		if finalRes.StatusCode == 403 || finalRes.StatusCode == 406 {
			return "", errors.New(WAF_BANNED_ERROR)
		}
		redirectCount++
	}

	// Extract the initial session cookie from the cookie jar
	bugcrowdURL, _ := url.Parse("https://bugcrowd.com")
	initialSessionCookie := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(bugcrowdURL) {
		if cookie.Name == bugcrowdSessionCookieName && cookie.Value != "" {
			initialSessionCookie = cookie.Value
			break
		}
	}
	if initialSessionCookie == "" {
		return "", errors.New("initial session cookie not found after redirects")
	}

	// Parse HTML to find OktaSessionManagement div and extract csrfToken
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(finalRes.BodyString))
	if err != nil {
		return "", fmt.Errorf("failed to parse final response HTML: %v", err)
	}

	var csrfToken string
	var refreshSessionPath string
	doc.Find("div[data-react-class='OktaSessionManagement']").Each(func(i int, s *goquery.Selection) {
		props, exists := s.Attr("data-react-props")
		if exists {
			csrfToken = gjson.Get(props, "csrfToken").String()
			refreshSessionPath = gjson.Get(props, "refreshSessionPath").String()
		}
	})

	if csrfToken == "" {
		return "", errors.New("csrfToken not found in OktaSessionManagement div")
	}
	if refreshSessionPath == "" {
		return "", errors.New("refreshSessionPath not found in OktaSessionManagement div")
	}

	// Build the oidc_session_management URL
	oidcURL := "https://bugcrowd.com" + refreshSessionPath
	if strings.HasPrefix(refreshSessionPath, "http") {
		oidcURL = refreshSessionPath
	}

	// Calculate expiry timestamp (current time + 2 hours in milliseconds)
	expiryTimestamp := time.Now().Add(2 * time.Hour).UnixMilli()
	oidcBody := map[string]interface{}{
		"expiry": expiryTimestamp,
	}
	oidcBodyJSON, _ := json.Marshal(oidcBody)

	// Send PUT request to oidc_session_management endpoint
	oidcRes, err := rateLimitedSendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "PUT",
			URL:    oidcURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "User-Agent", Value: USER_AGENT},
				{Name: "Content-Type", Value: "application/json"},
				{Name: "X-Csrf-Token", Value: csrfToken},
				{Name: "Cookie", Value: buildSessionCookieHeader(initialSessionCookie)},
				{Name: "Referer", Value: currentURL},
				{Name: "Origin", Value: "https://bugcrowd.com"},
			},
			Body: string(oidcBodyJSON),
		}, retryClient)
	if err != nil {
		return "", fmt.Errorf("failed to call oidc_session_management: %v", err)
	}
	if oidcRes.StatusCode == 403 || oidcRes.StatusCode == 406 {
		return "", errors.New(WAF_BANNED_ERROR)
	}
	if oidcRes.StatusCode >= 400 {
		return "", fmt.Errorf("oidc_session_management returned error %d: %s", oidcRes.StatusCode, oidcRes.BodyString)
	}

	// Extract the final session cookie from the response
	finalSessionCookie := ""
	for _, cookie := range retryClient.HTTPClient.Jar.Cookies(bugcrowdURL) {
		if cookie.Name == bugcrowdSessionCookieName && cookie.Value != "" {
			finalSessionCookie = cookie.Value
			break
		}
	}
	if finalSessionCookie == "" {
		return "", errors.New("final session cookie not found after oidc_session_management call")
	}

	utils.Log.Info("Login OK. Fetching programs, please wait...")
	utils.Log.Debug(fmt.Sprintf("Using %s cookie for session", bugcrowdSessionCookieName))
	return finalSessionCookie, nil
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
		utils.Log.Warn("Compliance required! Empty Extraction URL (Skipping): ", pData.Url)
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

func buildSessionCookieHeader(token string) string {
	return fmt.Sprintf("_crowdcontrol_session_key=%s", token)
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
	return authenticatorType == "password" || authenticatorKey == "password"
}

func extractOktaDataJSON(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	var jsonData string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		if jsonData != "" {
			return
		}
		text := s.Text()
		if strings.Contains(text, "var oktaData =") {
			// Extract the JSON object
			// var oktaData = { ... };
			re := regexp.MustCompile(`var oktaData = (\{[\s\S]*?\});`)
			match := re.FindStringSubmatch(text)
			if len(match) > 1 {
				jsonData = match[1]
				// Fix \x escapes to make it valid JSON for gjson
				// \x3A -> \u003A
				hexRe := regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)
				jsonData = hexRe.ReplaceAllString(jsonData, `\u00$1`)
			}
		}
	})
	return jsonData
}
