package hackerone

import (
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

func getProgramScope(authorization string, id string, bbpOnly bool, categories []string, includeOOS bool) (pData scope.ProgramData, err error) {
	pData.Url = "https://hackerone.com/" + id
	currentPageURL := "https://api.hackerone.com/v1/hackers/programs/" + id + "/structured_scopes?page%5Bnumber%5D=1&page%5Bsize%5D=100"

	// loop through pages
	for {
		var res *whttp.WHTTPRes
		var err error
		retries := 3
		var statusCode int

		var l int
		for retries > 0 {
			res, err = whttp.SendHTTPRequest(
				&whttp.WHTTPReq{
					Method: "GET",
					URL:    currentPageURL,
					Headers: []whttp.WHTTPHeader{
						{Name: "Authorization", Value: "Basic " + authorization},
					},
				}, nil)

			// retry if there was an http error or we didn't get the JSON we expected
			if err != nil || !strings.Contains(res.BodyString, "\"data\":") {
				retries--
				time.Sleep(2 * time.Second) // wait before retrying
				continue
			}

			break
		}

		if retries == 0 {
			return scope.ProgramData{}, fmt.Errorf("failed to retrieve data for id %s after 3 attempts with status %d", id, statusCode)
		}

		l = int(gjson.Get(res.BodyString, "data.#").Int())

		isDumpAll := categories == nil
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

				eligibleForBounty := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()
				eligibleForSubmission := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool()

				if eligibleForSubmission {
					if !bbpOnly || (bbpOnly && eligibleForBounty) {
						pData.InScope = append(pData.InScope, scope.ScopeElement{
							Target:      gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_identifier").Str,
							Description: strings.ReplaceAll(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  "),
							Category:    gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_type").Str,
						})
					}
				} else {
					if includeOOS {
						pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
							Target:      gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_identifier").Str,
							Description: strings.ReplaceAll(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  "),
							Category:    gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_type").Str,
						})
					}
				}
			}
		}

		// only print OOS with bbpOnly if at least one in-scope, paid, element was found
		if bbpOnly && len(pData.InScope) == 0 {
			pData.OutOfScope = []scope.ScopeElement{}
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

	return pData, nil
}

func getCategories(input string) []string {

	if strings.ToLower(input) == "all" {
		return nil // isDumpAll
	}

	categories := map[string][]string{
		"url":        {"URL", "WILDCARD", "IP_ADDRESS"},
		"cidr":       {"CIDR"},
		"mobile":     {"GOOGLE_PLAY_APP_ID", "OTHER_APK", "APPLE_STORE_APP_ID"},
		"android":    {"GOOGLE_PLAY_APP_ID", "OTHER_APK"},
		"apple":      {"APPLE_STORE_APP_ID", "TESTFLIGHT"},
		"ai":         {"AI_MODEL"},
		"other":      {"OTHER"},
		"hardware":   {"HARDWARE"},
		"code":       {"SOURCE_CODE", "SMART_CONTRACT"},
		"executable": {"DOWNLOADABLE_EXECUTABLES", "WINDOWS_APP_STORE_APP_ID"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]

	if !ok {
		utils.Log.Fatal("Invalid category selected: ", input)
	}

	return selectedCategory
}

func getProgramHandles(authorization string, pvtOnly bool, publicOnly bool, active bool, bbpOnly bool) (handles []string) {
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
			utils.Log.Warn("HTTP request failed: ", err)
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
							if bbpOnly {
								if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.offers_bounties").Bool() == true {
									handles = append(handles, handle.Str)
								}
							} else {
								handles = append(handles, handle.Str)
							}
						}
					} else {
						handles = append(handles, handle.Str)
					}
				}
			} else {
				if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.state").Str == "public_mode" {
					if active {
						if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.submission_state").Str == "open" {
							if bbpOnly {
								if gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.offers_bounties").Bool() == true {
									handles = append(handles, handle.Str)
								}
							} else {
								handles = append(handles, handle.Str)
							}
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

func GetAllProgramsScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, active bool, concurrency int, printRealTime bool, outputFlags string, delimiter string, includeOOS bool) (programs []scope.ProgramData, err error) {
	utils.Log.Debug("Fetching list of program handles")
	programHandles := getProgramHandles(authorization, pvtOnly, publicOnly, active, bbpOnly)

	utils.Log.Debug("Fetching scope of each program. Concurrency: ", concurrency)
	ids := make(chan string, concurrency)
	errors := make(chan error, concurrency) // Channel to collect errors
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

				programData, err := getProgramScope(authorization, id, bbpOnly, getCategories(categories), includeOOS)

				if err != nil {
					utils.Log.Warn("Error fetching program scope: ", err)
					errors <- err
					continue
				}

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
	close(errors) // Close the errors channel after all goroutines are done

	// Check if there were any errors
	for err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return programs, nil
}

func HacktivityMonitor(pages int) {
	for pageID := 0; pageID < pages; pageID++ {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "POST",
				URL:    "https://hackerone.com/graphql",
				Headers: []whttp.WHTTPHeader{
					{Name: "Content-Type", Value: "application/json"},
				},
				Body: `{"operationName":"CompleteHacktivitySearchQuery","variables":{"userPrompt":null,"queryString":"disclosed:false","size":100,"from":` + strconv.Itoa(pageID*100) + `,"sort":{"field":"latest_disclosable_activity_at","direction":"DESC"},"product_area":"hacktivity","product_feature":"overview"},"query":"query CompleteHacktivitySearchQuery($queryString: String!, $from: Int, $size: Int, $sort: SortInput!) {\n  me {\n    id\n    __typename\n  }\n  search(\n    index: CompleteHacktivityReportIndexService\n    query_string: $queryString\n    from: $from\n    size: $size\n    sort: $sort\n  ) {\n    __typename\n    total_count\n    nodes {\n      __typename\n      ... on CompleteHacktivityReportDocument {\n        id\n        _id\n        reporter {\n          id\n          name\n          username\n          ...UserLinkWithMiniProfile\n          __typename\n        }\n        cve_ids\n        cwe\n        severity_rating\n        upvoted: upvoted_by_current_user\n        public\n        report {\n          id\n          databaseId: _id\n          title\n          substate\n          url\n          disclosed_at\n          report_generated_content {\n            id\n            hacktivity_summary\n            __typename\n          }\n          __typename\n        }\n        votes\n        team {\n          handle\n          name\n          medium_profile_picture: profile_picture(size: medium)\n          url\n          id\n          currency\n          ...TeamLinkWithMiniProfile\n          __typename\n        }\n        total_awarded_amount\n        latest_disclosable_action\n        latest_disclosable_activity_at\n        submitted_at\n        disclosed\n        has_collaboration\n        __typename\n      }\n    }\n  }\n}\n\nfragment UserLinkWithMiniProfile on User {\n  id\n  username\n  __typename\n}\n\nfragment TeamLinkWithMiniProfile on Team {\n  id\n  handle\n  name\n  __typename\n}\n"}`,
			}, nil)

		if err != nil {
			utils.Log.Warn("HTTP request failed: ", err)
		}

		if res.StatusCode != 200 {
			utils.Log.Fatal("Wrong status code. Got: ", res.StatusCode)
		}

		// Parse and iterate over each element in the nodes array
		// Parse and iterate over each element in the nodes array
		gjson.Get(res.BodyString, "data.search.nodes").ForEach(func(key, value gjson.Result) bool {
			// Extract the fields you are interested in
			rawReportID := value.Get("id").String()
			programHandle := value.Get("team.handle").String()
			reporterUsername := value.Get("reporter.username").String()
			latestAction := value.Get("latest_disclosable_action").String()
			latestActivityAt := value.Get("latest_disclosable_activity_at").String()
			submittedAt := value.Get("submitted_at").String()

			// Decode the report ID
			decodedBytes, err := base64.StdEncoding.DecodeString(rawReportID)
			if err != nil {
				log.Fatalf("Failed to decode base64 string: %v", err)
			}
			decodedStr := string(decodedBytes)
			parts := strings.Split(decodedStr, "/")
			if len(parts) == 0 {
				log.Fatal("No '/' found in decoded string")
			}
			reportID := parts[len(parts)-1]

			// Convert date strings to human-friendly format
			latestActivityAtFormatted := formatDate(latestActivityAt)
			submittedAtFormatted := formatDate(submittedAt)

			// Print the extracted data
			fmt.Printf("ID: %s, program: %s, reporter: %s, latest_action: %s, latest_activity: %s, submitted: %s\n",
				reportID, programHandle, reporterUsername, latestAction, latestActivityAtFormatted, submittedAtFormatted)

			return true // Continue iterating
		})
	}
}

func formatDate(dateStr string) string {
	layout := "2006-01-02T15:04:05.000Z" // ISO 8601 format
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		log.Printf("Error parsing date: %v", err)
		return dateStr // Return original string if parsing fails
	}
	return t.Format("02/01/2006 15:04:05") // Format as dd/mm/yyyy h:m:s
}
