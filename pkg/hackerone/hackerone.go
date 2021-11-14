package hackerone

import (
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/sw33tLie/bbscope/internal/scope"
	"github.com/tidwall/gjson"
)

const (
	USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
)

func getProgramScope(authorization string, id string, bbpOnly bool, categories []string) (pData scope.ProgramData) {

	req, err := http.NewRequest("GET", "https://api.hackerone.com/v1/hackers/programs/"+id, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Authorization", "Basic "+authorization)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	pData.Url = "https://hackerone.com/" + gjson.Get(string(body), "attributes.handle").Str

	l := int(gjson.Get(string(body), "relationships.structured_scopes.data.#").Int())
	for i := 0; i < l; i++ {
		catFound := false
		assetCategory := gjson.Get(string(body), "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.asset_type").Str

		for _, cat := range categories {
			if cat == assetCategory {
				catFound = true
				break
			}
		}
		if catFound {
			// If it's in the in-scope table (and not in the OOS one)
			if gjson.Get(string(body), "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool() {
				if !bbpOnly || (bbpOnly && gjson.Get(string(body), "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()) {
					pData.InScope = append(pData.InScope, scope.ScopeElement{
						Target:      gjson.Get(string(body), "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.asset_identifier").Str,
						Description: strings.ReplaceAll(gjson.Get(string(body), "relationships.structured_scopes.data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  "),
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
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func getProgramHandles(authorization string, pvtOnly bool, publicOnly bool, active bool) (handles []string) {
	currentURL := "https://api.hackerone.com/v1/hackers/programs"
	for {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			log.Fatal(err)
		}

		req.Header.Set("User-Agent", USER_AGENT)
		req.Header.Set("Authorization", "Basic "+authorization)

		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if strings.Contains(string(body), ":401}") {
			log.Fatal("Invalid username or token")
		}

		for i := 0; i < int(gjson.Get(string(body), "data.#").Int()); i++ {
			handle := gjson.Get(string(body), "data."+strconv.Itoa(i)+".attributes.handle")

			if !publicOnly {
				if !pvtOnly || (pvtOnly && gjson.Get(string(body), "data."+strconv.Itoa(i)+".attributes.state").Str == "soft_launched") {
					if active {
						if gjson.Get(string(body), "data."+strconv.Itoa(i)+".attributes.submission_state").Str == "open" {
							handles = append(handles, handle.Str)
						}
					} else {
						handles = append(handles, handle.Str)
					}
				}
			} else {
				if gjson.Get(string(body), "data."+strconv.Itoa(i)+".attributes.state").Str == "public_mode" {
					if active {
						if gjson.Get(string(body), "data."+strconv.Itoa(i)+".attributes.submission_state").Str == "open" {
							handles = append(handles, handle.Str)
						}
					} else {
						handles = append(handles, handle.Str)
					}
				}
			}
		}

		currentURL = gjson.Get(string(body), "links.next").Str

		// We reached the end
		if currentURL == "" {
			break
		}
	}

	return handles
}

// GetAllProgramsScope xxx
func GetAllProgramsScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, active bool) (programs []scope.ProgramData) {
	programHandles := getProgramHandles(authorization, pvtOnly, publicOnly, active)
	threads := 50
	ids := make(chan string, threads)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(threads)

	for i := 0; i < threads; i++ {
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
func PrintAllScope(authorization string, bbpOnly bool, pvtOnly bool, publicOnly bool, categories string, outputFlags string, delimiter string, active bool) {
	programs := GetAllProgramsScope(authorization, bbpOnly, pvtOnly, publicOnly, categories, active)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter)
	}
}
