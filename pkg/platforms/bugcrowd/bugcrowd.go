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

	authenticityToken := ""
	bugcrowdURL, _ := url.Parse("https://bugcrowd.com")
	allCookies := retryClient.HTTPClient.Jar.Cookies(bugcrowdURL)
	for _, cookie := range allCookies {
		cookieName := strings.ToLower(cookie.Name)
		if cookieName == "csrf-token" || cookieName == "_csrf_token" || cookieName == "csrf_token" ||
			cookieName == "authenticity_token" || cookieName == "_authenticity_token" {
			authenticityToken = cookie.Value
			break
		}
	}
	if authenticityToken == "" {
		csrfHeader := signInRes.Headers.Get("X-CSRF-Token")
		if csrfHeader != "" {
			authenticityToken = csrfHeader
		}
	}
	if authenticityToken == "" {
		setCookies := signInRes.Headers.Values("Set-Cookie")
		for _, setCookie := range setCookies {
			if strings.Contains(strings.ToLower(setCookie), "csrf") || strings.Contains(strings.ToLower(setCookie), "authenticity") {
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
	if authenticityToken == "" && csrfTokenFromCookie != "" {
		authenticityToken = csrfTokenFromCookie
	}
	if authenticityToken == "" && docErr == nil && signInDoc != nil {
		signInDoc.Find("meta").Each(func(i int, s *goquery.Selection) {
			name, _ := s.Attr("name")
			content, exists := s.Attr("content")
			if exists && (name == "csrf-token" || name == "csrf_token" || name == "authenticity-token" || name == "authenticity_token") {
				authenticityToken = content
			}
		})
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
		if authenticityToken == "" {
			signInDoc.Find("script").Each(func(i int, s *goquery.Selection) {
				scriptContent := s.Text()
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
	if authenticityToken == "" {
		htmlSample := signInRes.BodyString
		if len(htmlSample) > 500 {
			htmlSample = htmlSample[:500]
		}
		return "", fmt.Errorf("authenticity token not found in sign_in page. HTML sample: %s", htmlSample)
	}

	headers := []whttp.WHTTPHeader{
		{Name: "User-Agent", Value: USER_AGENT},
		{Name: "Content-Type", Value: "application/x-www-form-urlencoded"},
		{Name: "Origin", Value: "https://bugcrowd.com"},
		{Name: "Referer", Value: signInURL},
		{Name: "X-Csrf-Token", Value: authenticityToken},
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
		oktaStateTokenFromPage = extractOktaStateToken(oktaAuthorizeRes.BodyString)
	} else if strings.Contains(authHackerRes.BodyString, "okta-sign-in") ||
		strings.Contains(authHackerRes.BodyString, "okta-signin-widget") {
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

	callbackURL := ""
	if tokenRedirectRes.StatusCode >= 300 && tokenRedirectRes.StatusCode < 400 {
		callbackURL = tokenRedirectRes.Headers.Get("Location")
	} else {
		re := regexp.MustCompile(`https://bugcrowd\.com/user/auth/hacker/callback[^"'\s]+`)
		matches := re.FindString(tokenRedirectRes.BodyString)
		if matches != "" {
			callbackURL = matches
		}
	}

	if callbackURL != "" {
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

		dashboardURL := ""
		if callbackRes.StatusCode >= 300 && callbackRes.StatusCode < 400 {
			dashboardURL = callbackRes.Headers.Get("Location")
		} else {
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
	}

	if sessionValue, sessionName := findSessionCookie(retryClient.HTTPClient.Jar, "https://bugcrowd.com"); sessionValue != "" {
		return logSessionSuccess(sessionName, sessionValue)
	}
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
	return authenticatorType == "password" || authenticatorKey == "password"
}
