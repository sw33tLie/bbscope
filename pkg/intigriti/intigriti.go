package intigriti

import (
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/sw33tLie/bbscope/internal/scope"
	"github.com/tidwall/gjson"
)

const (
	INTIGRITI_PROGRAMS_ENDPOINT     = "https://api.intigriti.com/core/researcher/program"
	INTIGRITI_PROGRAM_BASE_ENDPOINT = "https://api.intigriti.com/core/program"
	USER_AGENT                      = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
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
	req, err := http.NewRequest("GET", INTIGRITI_PROGRAM_BASE_ENDPOINT+"/"+companyHandle+"/"+programHandle, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", USER_AGENT)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	latestVersionIndex := len(gjson.Get(string(body), "domains.#.content").Array()) - 1
	currentContent := gjson.Get(string(body), "domains."+strconv.Itoa(latestVersionIndex)+".content")

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
	//return Program{companyHandle, programHandle, endpoints}
}

/*
// ListPrograms prints a list of available programs
func ListPrograms(token string, bbpOnly bool, pvtOnly bool) {
	req, err := http.NewRequest("GET", INTIGRITI_PROGRAMS_ENDPOINT, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", USER_AGENT)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	data := gjson.GetMany(string(body), "#.companyHandle", "#.handle", "#.maxBounty", "#.confidentialityLevel")

	allCompanyHandles := data[0].Array()
	allHandles := data[1].Array()
	allMinBounties := data[2].Array()
	confidentialityLevels := data[3].Array()

	for i := 0; i < len(allHandles); i++ {
		if !pvtOnly || (pvtOnly && confidentialityLevels[i].Int() == 1) {
			if !bbpOnly || (bbpOnly && allMinBounties[i].Float() > 0) {
				fmt.Println(strings.ReplaceAll("https://www.intigriti.com/researcher/programs/"+allCompanyHandles[i].Str+"/"+allHandles[i].Str+"/detail", " ", "%20"))
			}
		}
	}
}*/

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string) (programs []scope.ProgramData) {
	req, err := http.NewRequest("GET", INTIGRITI_PROGRAMS_ENDPOINT, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", USER_AGENT)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	data := gjson.GetMany(string(body), "#.companyHandle", "#.handle", "#.maxBounty", "#.confidentialityLevel")

	allCompanyHandles := data[0].Array()
	allHandles := data[1].Array()
	allMinBounties := data[2].Array()
	confidentialityLevels := data[3].Array()

	for i := 0; i < len(allHandles); i++ {
		if !pvtOnly || (pvtOnly && confidentialityLevels[i].Int() == 1) {
			if !bbpOnly || (bbpOnly && allMinBounties[i].Float() > 0) {
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
