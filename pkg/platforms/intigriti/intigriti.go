package intigriti

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

func GetCategoryID(input string) []int {
	categories := map[string][]int{
		"url":      {1},
		"cidr":     {4},
		"mobile":   {2, 3},
		"android":  {2},
		"apple":    {3},
		"device":   {5},
		"other":    {6},
		"wildcard": {7},
		"all":      {1, 2, 3, 4, 5, 6, 7},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetProgramScope(token string, programID string, categories string, bbpOnly bool, includeOOS bool) (pData scope.ProgramData) {
	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    "https://api.intigriti.com/external/researcher/v1/programs/" + programID,
			Headers: []whttp.WHTTPHeader{
				{Name: "Authorization", Value: "Bearer " + token},
			},
		}, nil)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	if res.StatusCode == 401 {
		utils.Log.Fatal("Invalid auth token")
	}

	if strings.Contains(res.BodyString, "Request blocked") {
		utils.Log.Info("Rate limited. Retrying...")
		time.Sleep(2 * time.Second)
		return GetProgramScope(token, programID, categories, bbpOnly, includeOOS)
	}

	// Use gjson to get the content array
	contentArray := gjson.Get(res.BodyString, "domains.content")

	// Iterate over each item in the array
	contentArray.ForEach(func(key, value gjson.Result) bool {
		endpoint := value.Get("endpoint").String()
		categoryID := value.Get("type.id").Int()
		categoryValue := value.Get("type.value").Str
		tierID := value.Get("tier.id").Int()
		description := value.Get("description").Str

		// Check if the tier ID is not 5 (out of scope)
		if tierID != 5 {
			if !bbpOnly || (bbpOnly && tierID != 1) {
				// Check if this element belongs to one of the categories the user chose
				if isInArray(int(categoryID), GetCategoryID(categories)) {
					pData.InScope = append(pData.InScope, scope.ScopeElement{
						Target:      endpoint,
						Description: strings.ReplaceAll(description, "\n", "  "),
						Category:    categoryValue,
					})
				}
			}
		} else {
			// TODO: This isn't being printed
			if includeOOS {
				pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
					Target:      endpoint,
					Description: strings.ReplaceAll(description, "\n", "  "),
					Category:    categoryValue,
				})
			}
		}

		return true // Keep iterating
	})

	return pData
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories, outputFlags, delimiterCharacter string, includeOOS, printRealTime bool) (programs []scope.ProgramData) {
	offset := 0
	limit := 500
	total := 0

	for {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    fmt.Sprintf("https://api.intigriti.com/external/researcher/v1/programs?statusId=3&limit=%d&offset=%d", limit, offset),
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Bearer " + token},
				},
			}, nil)

		if err != nil {
			utils.Log.Fatal("HTTP request failed: ", err)
		}

		if res.StatusCode == 401 {
			utils.Log.Fatal("Invalid auth token")
		}

		bodyString := string(res.BodyString)

		if offset == 0 {
			total = int(gjson.Get(bodyString, "maxCount").Int())
			utils.Log.Info("Total Programs available: ", total)
		}

		records := gjson.Get(bodyString, "records").Array()
		for _, record := range records {
			id := record.Get("id").String()
			maxBounty := record.Get("maxBounty.value").Int()
			confidentialityLevel := record.Get("confidentialityLevel.id").Int()
			programPath := strings.Split(record.Get("webLinks.detail").String(), "=")[1]

			// Types of confidentialityLevel: 1 InviteOnly, 2 Application, 3 Registered, 4 Public.
			// We assume privates are 1, 2 and 3.

			if (pvtOnly && confidentialityLevel != 4) || !pvtOnly {
				if (bbpOnly && maxBounty != 0) || !bbpOnly {
					pData := GetProgramScope(token, id, categories, bbpOnly, includeOOS)
					pData.Url = "https://app.intigriti.com/researcher" + programPath
					if printRealTime {
						scope.PrintProgramScope(pData, outputFlags, delimiterCharacter, includeOOS)
					}

					programs = append(programs, pData)
				}
			}
		}

		offset += len(records)
		if offset >= total {
			break
		}
	}

	return programs
}

// Function to check if an int is in a slice of ints
func isInArray(val int, array []int) bool {
	for _, item := range array {
		if item == val {
			return true
		}
	}
	return false
}
