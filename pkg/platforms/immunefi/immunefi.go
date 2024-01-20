package immunefi

import (
	"log"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/sw33tLie/bbscope/pkg/scope"
	"github.com/sw33tLie/bbscope/pkg/whttp"
	"github.com/tidwall/gjson"
)

const (
	PLATFORM_URL = "https://immunefi.com"
)

func PrintAllScope(categories, outputFlags, delimiter string, concurrency int) {
	programs := GetAllProgramsScope(categories, concurrency)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter, false)
	}
}

func getCategories(input string) []string {
	categories := map[string][]string{
		"web":       {"websites_and_applications"},
		"contracts": {"smart_contract"},
		"all":       {"websites_and_applications", "smart_contract"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetAllProgramsScope(categories string, concurrency int) (programs []scope.ProgramData) {

	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    PLATFORM_URL + "/explore/",
			Headers: []whttp.WHTTPHeader{
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))

	if err != nil {
		log.Fatal("Failed to parse HTML")
	}

	selectedCategories := getCategories(categories)

	var programURLs []string
	doc.Find("#__NEXT_DATA__").Each(func(index int, s *goquery.Selection) {
		json := s.Contents().Text()
		jsonPrograms := gjson.Get(json, "props.pageProps.bounties")

		for _, program := range jsonPrograms.Array() {
			programID := gjson.Get(program.Raw, "id")
			isExternal := gjson.Get(program.Raw, "is_external").Bool()

			if !isExternal {
				programURLs = append(programURLs, PLATFORM_URL+"/bounty/"+programID.Str+"/")
			}
		}
	})

	// Iterate over all program pages
	p := make(chan string, concurrency)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				url := <-p

				if url == "" {
					break
				}

				res, err := whttp.SendHTTPRequest(
					&whttp.WHTTPReq{
						Method: "GET",
						URL:    url,
						Headers: []whttp.WHTTPHeader{
							{Name: "Accept", Value: "*/*"},
						},
					}, nil)

				if err != nil {
					log.Fatal("HTTP request failed: ", err)
				}

				doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))

				if err != nil {
					log.Fatal("Failed to parse HTML")
				}

				doc.Find("#__NEXT_DATA__").Each(func(index int, s *goquery.Selection) {
					json := s.Contents().Text()
					jsonProgram := gjson.Get(json, "props.pageProps.bounty")
					var tempScope []scope.ScopeElement

					for _, scopeElement := range gjson.Get(jsonProgram.Raw, "assets").Array() {
						elementTarget := gjson.Get(scopeElement.Raw, "url").Str
						elementType := gjson.Get(scopeElement.Raw, "type").Str

						for _, currentCat := range selectedCategories {
							if currentCat == "websites_and_applications" && strings.Contains(elementType, "websites_and_applications") {
								tempScope = append(tempScope, scope.ScopeElement{
									Target:      elementTarget,
									Description: "",
									Category:    currentCat,
								})
							} else if currentCat == "smart_contract" && strings.Contains(elementType, "smart_contract") {
								tempScope = append(tempScope, scope.ScopeElement{
									Target:      elementTarget,
									Description: "",
									Category:    currentCat,
								})
							}
						}
					}

					programs = append(programs, scope.ProgramData{
						Url:        url,
						InScope:    tempScope,
						OutOfScope: nil,
					})
				})

			}
			processGroup.Done()
		}()
	}

	for _, pURL := range programURLs {
		p <- pURL
	}

	close(p)
	processGroup.Wait()
	return programs
}
