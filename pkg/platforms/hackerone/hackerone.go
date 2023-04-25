package hackerone

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	RATE_LIMIT_WAIT_TIME_SEC = 5
	RATE_LIMIT_MAX_RETRIES   = 10
	RATE_LIMIT_HTTP_STATUS   = 429
)

func getProgramScope(authorization string, id string, bbpOnly bool, categories []string) (pData scope.ProgramData) {
	var err error
	res := &whttp.WHTTPRes{}
	lastStatus := -1

	for i := 0; i < RATE_LIMIT_MAX_RETRIES; i++ {
		res, err = whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    "https://api.hackerone.com/v1/hackers/programs/" + id,
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Basic " + authorization},
				},
			}, http.DefaultClient)

		if err != nil {
			utils.Log.Warn("HTTP request failed: ", err, " Retrying...")
			time.Sleep(2 * time.Second)
			continue
		}

		lastStatus = res.StatusCode
		// exit the loop if we succeeded
		if res.StatusCode != RATE_LIMIT_HTTP_STATUS {
			break
		} else {
			// encountered rate limit
			time.Sleep(RATE_LIMIT_WAIT_TIME_SEC * time.Second)
		}
	}
	if lastStatus > 200 {
		// if we completed the requests with a final (non-429) status and we still failed
		utils.Log.Fatal("Could not retrieve data for id ", id, " with status ", lastStatus)
	}

	pData.Url = "https://hackerone.com/" + id

	l := int(gjson.Get(res.BodyString, "relationships.structured_scopes.data.#").Int())

	isDumpAll := len(categories) == len(getCategories("all"))
	for i := 0; i < l; i++ {

		catFound := false
		if !isDumpAll {
			assetCategory := gjson.Get(res.BodyString, "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.asset_type").Str

			for _, cat := range categories {
				if cat == assetCategory {
					catFound = true
					break
				}
			}
		}

		if catFound || isDumpAll {
			// If it's in the in-scope table (and not in the OOS one)
			if gjson.Get(res.BodyString, "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool() {
				if !bbpOnly || (bbpOnly && gjson.Get(res.BodyString, "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()) {
					pData.InScope = append(pData.InScope, scope.ScopeElement{
						Target:      gjson.Get(res.BodyString, "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.asset_identifier").Str,
						Description: strings.ReplaceAll(gjson.Get(res.BodyString, "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  "),
						Category:    "", // TODO
					})
				}
			}
		}
	}

	if l == 0 {
		pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE", Description: "", Category: ""})
	}

	return pData
}

func getCategories(input string) []string {
	categories := map[string][]string{
		"url":        {"URL"},
		"cidr":       {"CIDR"},
		"mobile":     {"GOOGLE_PLAY_APP_ID", "OTHER_APK", "APPLE_STORE_APP_ID"},
		"android":    {"GOOGLE_PLAY_APP_ID", "OTHER_APK"},
		"apple":      {"APPLE_STORE_APP_ID"},
		"other":      {"OTHER"},
		"hardware":   {"HARDWARE"},
		"code":       {"SOURCE_CODE"},
		"executable": {"DOWNLOADABLE_EXECUTABLES"},
		"all":        {"URL", "CIDR", "GOOGLE_PLAY_APP_ID", "OTHER_APK", "APPLE_STORE_APP_ID", "OTHER", "HARDWARE", "SOURCE_CODE", "DOWNLOADABLE_EXECUTABLES"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		utils.Log.Fatal("Invalid category selected")
	}
	return selectedCategory
}

func getProgramHandles(authorization string, pvtOnly bool, publicOnly bool, active bool) (handles []string) {
	currentURL := "https://api.hackerone.com/v1/hackers/programs"
	for {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    currentURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Basic " + authorization},
				},
			}, http.DefaultClient)

		if err != nil {
			utils.Log.Warn("HTTP request failed: ", err, " Retrying...")
			time.Sleep(2 * time.Second)
			continue
		}

		if res.StatusCode != 200 {
			utils.Log.Fatal("Fetching failed. Got status Code: ", res.StatusCode)
		}

		for i := 0; i < int(gjson.Get(res.BodyString, "data.#").Int()); i++ {
			handle := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.handle")

			if !publicOnly {
				if !pvtOnly || (pvtOnly && gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.state").Str == "soft_launched") {
					if active {
						if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.submission_state").Str == "open" {
							handles = append(handles, handle.Str)
						}
					} else {
						handles = append(handles, handle.Str)
					}
				}
			} else {
				if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.state").Str == "public_mode" {
					if active {
						if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.submission_state").Str == "open" {
							handles = append(handles, handle.Str)
						}
					} else {
						handles = append(handles, handle.Str)
					}
				}
			}
		}

		currentURL = gjson.Get(res.BodyString, "links.next").Str

		// We reached the end
		if currentURL == "" {
			break
		}
	}

	return handles
}

// GetAllProgramsScope xxx
func GetAllProgramsScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, active bool, concurrency int) (programs []scope.ProgramData) {
	utils.Log.Debug("Fetching list of program handles")
	programHandles := getProgramHandles(authorization, pvtOnly, publicOnly, active)

	utils.Log.Debug("Fetching scope of each program. Concurrency: ", concurrency)
	ids := make(chan string, concurrency)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				id := <-ids

				if id == "" {
					break
				}

				programs = append(programs, getProgramScope(authorization, id, bbpOnly, getCategories(categories)))
			}
			processGroup.Done()
		}()
	}

	for _, s := range programHandles {
		ids <- s
	}

	close(ids)
	processGroup.Wait()

	return programs
}

// PrintAllScope prints to stdout all scope elements of all targets
func PrintAllScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, outputFlags string, delimiter string, active bool, concurrency int) {
	programs := GetAllProgramsScope(authorization, bbpOnly, pvtOnly, publicOnly, categories, active, concurrency)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter)
	}
}
