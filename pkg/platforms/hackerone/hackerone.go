package hackerone

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

func getProgramScope(authorization string, id string, bbpOnly bool, categories []string) (pData scope.ProgramData) {
	pData.Url = "https://hackerone.com/" + id
	currentPageURL := "https://api.hackerone.com/v1/hackers/programs/" + id + "/structured_scopes?page%5Bnumber%5D=1&page%5Bsize%5D=100"

	// loop through pages
	for {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    currentPageURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Basic " + authorization},
				},
			}, nil)

		if err != nil {
			utils.Log.Warn("HTTP request failed: ", err, " Retrying...")
		}

		if res.StatusCode != 200 {
			// if we completed the requests with a final (non-429) status and we still failed
			utils.Log.Fatal("Could not retrieve data for id ", id, " with status ", res.StatusCode)
		}

		l := int(gjson.Get(res.BodyString, "data.#").Int())

		isDumpAll := len(categories) == len(getCategories("all"))
		for i := 0; i < l; i++ {

			catFound := false
			if !isDumpAll {
				assetCategory := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_type").Str

				for _, cat := range categories {
					if cat == assetCategory {
						catFound = true
						break
					}
				}
			}

			if catFound || isDumpAll {
				// If it's in the in-scope table (and not in the OOS one)
				if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool() {
					if !bbpOnly || (bbpOnly && gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()) {
						pData.InScope = append(pData.InScope, scope.ScopeElement{
							Target:      gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_identifier").Str,
							Description: strings.ReplaceAll(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  "),
							Category:    "", // TODO
						})
					}
				}
			}
		}

		if l == 0 {
			pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE", Description: "", Category: ""})
		}

		nextPageURL := gjson.Get(res.BodyString, "links.next")
		if nextPageURL.Exists() {
			currentPageURL = nextPageURL.String()
		} else {
			break // no more pages
		}
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
	currentURL := "https://api.hackerone.com/v1/hackers/programs?page%5Bsize%5D=100"
	for {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    currentURL,
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Basic " + authorization},
				},
			}, nil)

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

func GetAllProgramsScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, active bool, concurrency int, printRealTime bool, outputFlags string, delimiter string, includeOOS bool) (programs []scope.ProgramData) {
	utils.Log.Debug("Fetching list of program handles")
	programHandles := getProgramHandles(authorization, pvtOnly, publicOnly, active)

	utils.Log.Debug("Fetching scope of each program. Concurrency: ", concurrency)
	ids := make(chan string, concurrency)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(concurrency)

	// Define a mutex
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				id, more := <-ids
				if !more {
					break
				}

				programData := getProgramScope(authorization, id, bbpOnly, getCategories(categories))

				mu.Lock()
				programs = append(programs, programData)

				// Check if printRealTime is true and print scope
				if printRealTime {
					scope.PrintProgramScope(programData, outputFlags, delimiter, includeOOS)
				}

				mu.Unlock()
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
