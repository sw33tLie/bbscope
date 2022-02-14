package immunefi

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/tidwall/gjson"
)

const (
	USER_AGENT   = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
	PLATFORM_URL = "https://immunefi.com"
)

func PrintAllScope(categories, outputFlags, delimiter string) {
	programs := GetAllProgramsScope(categories)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter)
	}
}

func getCategories(input string) []string {
	categories := map[string][]string{
		"web":       {"Web"},
		"contracts": {"Smart Contract"},
		"all":       {"Web", "Smart Contract"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetAllProgramsScope(categories string) (programs []scope.ProgramData) {
	req, err := http.NewRequest("GET", PLATFORM_URL+"/explore/", nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Accept", "*/*")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))

	if err != nil {
		log.Fatal("Failed to parse HTML")
	}

	selectedCategories := getCategories(categories)

	doc.Find("#__NEXT_DATA__").Each(func(index int, s *goquery.Selection) {
		json := s.Contents().Text()
		jsonPrograms := gjson.Get(json, "props.pageProps.bounties")

		for _, program := range jsonPrograms.Array() {
			var tempScope []scope.ScopeElement

			programID := gjson.Get(program.Raw, "id")

			for _, scopeElement := range gjson.Get(program.Raw, "assets_in_scope").Array() {
				elementTarget := gjson.Get(scopeElement.Raw, "target").Str
				elementType := gjson.Get(scopeElement.Raw, "type").Str

				for _, currentCat := range selectedCategories {
					if currentCat == "Web" && strings.Contains(elementType, "Web") {
						tempScope = append(tempScope, scope.ScopeElement{
							Target:      elementTarget,
							Description: "",
							Category:    currentCat,
						})
					} else if currentCat == "Smart Contract" && strings.Contains(elementType, "Smart Contract") {
						tempScope = append(tempScope, scope.ScopeElement{
							Target:      elementTarget,
							Description: "",
							Category:    currentCat,
						})
					}
				}
			}

			programs = append(programs, scope.ProgramData{
				Url:        PLATFORM_URL + "/bounty/" + programID.Str,
				InScope:    tempScope,
				OutOfScope: nil,
			})
		}
	})

	return programs
}
