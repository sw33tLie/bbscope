package yeswehack

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	YESWEHACK_PROGRAMS_ENDPOINT     = "https://api.yeswehack.com/programs" // ?page=1
	YESWEHACK_PROGRAM_BASE_ENDPOINT = "https://api.yeswehack.com/programs/"
)

func GetCategoryID(input string) []string {
	categories := map[string][]string{
		"url":        {"web-application", "api", "ip-address"},
		"mobile":     {"mobile-application", "mobile-application-android", "mobile-application-ios"},
		"android":    {"mobile-application-android"},
		"apple":      {"mobile-application-ios"},
		"other":      {"other"},
		"executable": {"application"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetProgramScope(token string, companySlug string, categories string) (pData scope.ProgramData) {
	pData.Url = YESWEHACK_PROGRAM_BASE_ENDPOINT + companySlug

	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    pData.Url,
			Headers: []whttp.WHTTPHeader{
				{Name: "Authorization", Value: "Bearer " + token},
			},
		}, nil)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	chunkData := gjson.GetMany(res.BodyString, "scopes.#.scope", "scopes.#.scope_type")

	for i := 0; i < len(chunkData[0].Array()); i++ {
		// Skip category filtering if "all" is specified
		if strings.ToLower(categories) == "all" {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:      chunkData[0].Array()[i].Str,
				Description: "",
				Category:    chunkData[1].Array()[i].Str,
			})
			continue
		}

		selectedCatIDs := GetCategoryID(categories)
		catMatches := false
		for _, cat := range selectedCatIDs {
			if cat == chunkData[1].Array()[i].Str {
				catMatches = true
				break
			}
		}

		if catMatches {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:      chunkData[0].Array()[i].Str,
				Description: "",
				Category:    chunkData[1].Array()[i].Str,
			})
		}
	}

	return pData
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, delimiter string, printRealTime bool) (programs []scope.ProgramData) {
	var page = 1
	var nb_pages = 2

	for page <= nb_pages {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    YESWEHACK_PROGRAMS_ENDPOINT + "?page=" + strconv.Itoa(page),
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Bearer " + token},
				},
			}, nil)

		if err != nil {
			log.Fatal("HTTP request failed: ", err)
		}

		data := gjson.GetMany(res.BodyString, "items.#.slug", "items.#.bounty", "items.#.public")

		allCompanySlugs := data[0].Array()
		allRewarding := data[1].Array()
		allPublic := data[2].Array()

		for i := 0; i < len(allCompanySlugs); i++ {
			if !pvtOnly || (pvtOnly && !allPublic[i].Bool()) {
				if !bbpOnly || (bbpOnly && allRewarding[i].Bool()) {
					pData := GetProgramScope(token, allCompanySlugs[i].Str, categories)
					programs = append(programs, pData)

					if printRealTime {
						scope.PrintProgramScope(pData, outputFlags, delimiter, false)
					}
				}
			}
		}

		nb_pages = int(gjson.Get(res.BodyString, "pagination.nb_pages").Int())
		page += 1
	}

	return programs
}

func Login(email string, password, otpFetchCommand string) (string, error) {
	// Step 1: Send POST request to /login with email and password
	loginURL := "https://api.yeswehack.com/login"
	loginPayload := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	loginRes, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "POST",
			URL:    loginURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Content-Type", Value: "application/json"},
			},
			Body: loginPayload,
		}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to send login request: %v", err)
	}

	if loginRes.StatusCode != 200 {
		return "", fmt.Errorf("login failed with status code: %d", loginRes.StatusCode)
	}

	// Check if we got a direct token (no 2FA required)
	if directToken := gjson.Get(loginRes.BodyString, "token").String(); directToken != "" {
		return directToken, nil
	}

	// If no direct token, check for totp_token (2FA required)
	totpToken := gjson.Get(loginRes.BodyString, "totp_token").String()
	if totpToken == "" {
		return "", fmt.Errorf("invalid login response: neither token nor totp_token found")
	}

	// 2FA flow continues...
	if otpFetchCommand == "" {
		return "", fmt.Errorf("2FA is enabled but no OTP fetch command provided")
	}

	// Try OTP verification
	OTP_ATTEMPTS := 5
	for attempts := 1; attempts <= OTP_ATTEMPTS; attempts++ {
		// Obtain the 2FA code by running the system command using shell
		cmd := exec.Command("sh", "-c", otpFetchCommand)
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to execute 2FA command: %v", err)
		}

		code := strings.TrimSpace(string(output))
		if code == "" {
			return "", fmt.Errorf("2FA code is empty")
		}

		// Send POST request to /account/totp with totp_token and code
		totpURL := "https://api.yeswehack.com/account/totp"
		totpPayload := fmt.Sprintf(`{"token":"%s","code":"%s"}`, totpToken, code)

		totpRes, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "POST",
				URL:    totpURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "Content-Type", Value: "application/json"},
				},
				Body: totpPayload,
			}, nil)

		if err != nil {
			return "", fmt.Errorf("failed to send TOTP request: %v", err)
		}

		// If status code is not 400, break the retry loop
		if totpRes.StatusCode != 400 {
			if totpRes.StatusCode != 200 {
				return "", fmt.Errorf("TOTP verification failed with status code: %d", totpRes.StatusCode)
			}

			// Parse the response to get the final token
			finalToken := gjson.Get(totpRes.BodyString, "token").String()
			if finalToken == "" {
				return "", fmt.Errorf("final token not found in TOTP response")
			}

			return finalToken, nil
		}

		time.Sleep(5 * time.Second)
		// If this was the last attempt, return error
		if attempts == OTP_ATTEMPTS {
			return "", fmt.Errorf("TOTP verification failed after %d attempts", OTP_ATTEMPTS)
		}
	}

	return "", fmt.Errorf("unexpected error in TOTP verification")
}
