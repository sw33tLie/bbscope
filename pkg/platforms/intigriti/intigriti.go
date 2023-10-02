package intigriti

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	INTIGRITI_PROGRAMS_ENDPOINT = "https://api.intigriti.com/core/researcher/programs"
)

func GetCategoryID(input string) []int {
	categories := map[string][]int{
		"url":     {1},
		"cidr":    {4},
		"mobile":  {2, 3},
		"android": {2},
		"apple":   {3},
		"device":  {5},
		"other":   {6},
		"all":     {1, 2, 3, 4, 5, 6},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetProgramScope(token string, companyHandle string, programHandle string, categories string) (pData scope.ProgramData) {
	pData.Url = strings.ReplaceAll("https://www.intigriti.com/researcher/programs/"+companyHandle+"/"+programHandle+"/detail", " ", "%20")

	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    INTIGRITI_PROGRAMS_ENDPOINT + "/" + companyHandle + "/" + programHandle,
			Headers: []whttp.WHTTPHeader{
				{Name: "Authorization", Value: "Bearer " + token},
			},
		}, http.DefaultClient)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	latestVersionIndex := len(gjson.Get(res.BodyString, "domains.#.content").Array()) - 1
	currentContent := gjson.Get(res.BodyString, "domains."+strconv.Itoa(latestVersionIndex)+".content")

	chunkData := gjson.GetMany(currentContent.Raw, "#.endpoint", "#.type", "#.description")
	for i := 0; i < len(chunkData[0].Array()); i++ {
		selectedCatIDs := GetCategoryID(categories)

		catMatches := false
		for _, cat := range selectedCatIDs {
			if cat == int(chunkData[1].Array()[i].Int()) {
				catMatches = true
				break
			}
		}
		if catMatches {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:      chunkData[0].Array()[i].Str,
				Description: strings.ReplaceAll(chunkData[2].Array()[i].Str, "\n", "  "),
				Category:    "",
			})
		}
	}

	if len(pData.InScope) == 0 {
		pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE", Description: "", Category: ""})
	}

	return pData
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string) (programs []scope.ProgramData) {
	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    INTIGRITI_PROGRAMS_ENDPOINT,
			Headers: []whttp.WHTTPHeader{
				{Name: "Authorization", Value: "Bearer " + token},
			},
		}, http.DefaultClient)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	data := gjson.GetMany(res.BodyString, "#(type==1)#.companyHandle", "#(type==1)#.handle", "#(type==1)#.maxBounty.value", "#(type==1)#.confidentialityLevel")

	allCompanyHandles := data[0].Array()
	allHandles := data[1].Array()
	allMaxBounties := data[2].Array()

	confidentialityLevels := data[3].Array()

	for i := 0; i < len(allHandles); i++ {
		if !pvtOnly || (pvtOnly && confidentialityLevels[i].Int() == 1) {
			if !bbpOnly || (bbpOnly && allMaxBounties[i].Float() != 0) {
				pData := GetProgramScope(token, allCompanyHandles[i].Str, allHandles[i].Str, categories)
				programs = append(programs, pData)
			}
		}
	}

	return programs
}

func PrintAllScope(token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, delimiter string) {
	programs := GetAllProgramsScope(token, bbpOnly, pvtOnly, categories)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter)
	}
}
