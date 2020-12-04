package bugcrowd

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

func GetProgramPagePaths(sessionToken string, bbpOnly bool, pvtOnly bool) []string {
	allProgramsCount := 0
	currentProgramIndex := 0
	listEndpointURL := "https://bugcrowd.com/programs.json?"
	if pvtOnly {
		listEndpointURL = listEndpointURL + "accepted_invite[]=true&"
	}
	if bbpOnly {
		listEndpointURL = listEndpointURL + "points_only[]=false&"
	}
	listEndpointURL = listEndpointURL + "hidden[]=false&sort[]=invited-desc&sort[]=promoted-desc&offset[]="
	paths := []string{}

	for {
		req, err := http.NewRequest("GET", listEndpointURL+strconv.Itoa(currentProgramIndex), nil)
		if err != nil {
			log.Fatal(err)
		}

		req.Header.Set("Cookie", "_crowdcontrol_session="+sessionToken)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if allProgramsCount == 0 {
			allProgramsCount = int(gjson.Get(string(body), "meta.totalHits").Int())
		}

		chunkData := gjson.Get(string(body), "programs.#.program_url")
		for i := 0; i < len(chunkData.Array()); i++ {
			paths = append(paths, chunkData.Array()[i].Str)
		}
		currentProgramIndex += 25

		if allProgramsCount <= currentProgramIndex {
			break
		}
	}

	return paths
}

func PrintProgramScope(url string, token string, categories string, urlsToo bool) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Cookie", "_crowdcontrol_session="+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	// Yeah, HTML parsing is a pain @arcwhite do something damn it :D
	// Or at least, don't break this tool aka don't change HTML stuff <3

	var scope []string
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		fmt.Println("No url found")
		log.Fatal(err)
	}

	doc.Find("#user-guides__bounty-brief__targets-table").Each(func(index int, tablehtml *goquery.Selection) {
		tablehtml.Find("tr").Each(func(indextr int, rowhtml *goquery.Selection) {
			var currentTarget struct {
				line     string
				category string
			}

			rowhtml.Find("tbody td").Each(func(indexth int, tablecell *goquery.Selection) {
				if indexth == 0 {
					if urlsToo {
						currentTarget.line = strings.TrimSpace(tablecell.Text()) + " " + url
					} else {
						currentTarget.line = strings.TrimSpace(tablecell.Text())
					}
				} else if indexth == 1 {
					currentTarget.category = strings.TrimSpace(tablecell.Text())
				}
			})

			catMatches := false
			for _, cat := range GetCategories(categories) {
				if cat == currentTarget.category {
					catMatches = true
					break
				}
			}
			if catMatches {
				scope = append(scope, currentTarget.line)
			}
		})
	})

	for _, s := range scope {
		fmt.Println(s)
	}
}

func GetCategories(input string) []string {
	categories := map[string][]string{
		"url":      []string{"Website Testing"},
		"api":      []string{"API Testing"},
		"mobile":   []string{"Android", "iOS"},
		"android":  []string{"Android"},
		"apple":    []string{"iOS"},
		"other":    []string{"Other"},
		"hardware": []string{"Hardware Testing"},
		"all":      []string{"Website Testing", "API Testing", "Android", "iOS", "Other", "Hardware Testing"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

// GetScope fetches the scope for all programs
func GetScope(token string, bbpOnly bool, pvtOnly bool, categories string, urlsToo bool, concurrency int) {
	programPaths := GetProgramPagePaths(token, pvtOnly, bbpOnly)

	urls := make(chan string, concurrency)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			for {
				url := <-urls

				if url == "" {
					break
				}

				PrintProgramScope(url, token, categories, urlsToo)
			}
			processGroup.Done()
		}()
	}

	for _, path := range programPaths {
		urls <- "https://bugcrowd.com" + path
	}

	close(urls)
	processGroup.Wait()

}

// ListPrograms prints a list of available programs
func ListPrograms(token string, bbpOnly bool, pvtOnly bool) {
	programPaths := GetProgramPagePaths(token, bbpOnly, pvtOnly)
	for _, path := range programPaths {
		fmt.Println("https://bugcrowd.com" + path)
	}
}
