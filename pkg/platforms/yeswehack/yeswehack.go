package yeswehack

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
	YESWEHACK_PROGRAMS_ENDPOINT     = "https://api.yeswehack.com/programs" // ?page=1
	YESWEHACK_PROGRAM_BASE_ENDPOINT = "https://api.yeswehack.com/programs/"
)

func GetCategoryID(input string) []string {
	categories := map[string][]string{
		"url":        {"web-application", "api", "ip-address"},
		"mobile":     {"mobile-application", "mobile-application-android", "mobile-application-ios"},
		"android":    {"mobile-application-android"},
		"apple":      {"mobile-application-ios"},
		"other":      {"other"},
		"executable": {"application"},
		"all":        {"web-application", "api", "ip-address", "mobile-application", "mobile-application-android", "mobile-application-ios", "other", "application"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

func GetProgramScope(token string, companySlug string, categories string) (pData scope.ProgramData) {
	pData.Url = YESWEHACK_PROGRAM_BASE_ENDPOINT + companySlug

	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    pData.Url,
			Headers: []whttp.WHTTPHeader{
				{Name: "Authorization", Value: "Bearer " + token},
			},
		}, http.DefaultClient)

	if err != nil {
		log.Fatal("HTTP request failed: ", err)
	}

	chunkData := gjson.GetMany(res.BodyString, "scopes.#.scope", "scopes.#.scope_type")

	for i := 0; i < len(chunkData[0].Array()); i++ {
		selectedCatIDs := GetCategoryID(categories)

		catMatches := false
		for _, cat := range selectedCatIDs {
			if cat == chunkData[1].Array()[i].Str {
				catMatches = true
				break
			}
		}

		if catMatches {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:      chunkData[0].Array()[i].Str,
				Description: "",
				Category:    chunkData[1].Array()[i].Str,
			})
		}
	}

	return pData
}

func GetAllProgramsScope(token string, bbpOnly bool, pvtOnly bool, categories string) (programs []scope.ProgramData) {

	var page = 1
	var nb_pages = 2

	for page <= nb_pages {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    YESWEHACK_PROGRAMS_ENDPOINT + "?page=" + strconv.Itoa(page),
				Headers: []whttp.WHTTPHeader{
					{Name: "Authorization", Value: "Bearer " + token},
				},
			}, http.DefaultClient)

		if err != nil {
			log.Fatal("HTTP request failed: ", err)
		}

		data := gjson.GetMany(res.BodyString, "items.#.slug", "items.#.bounty", "items.#.public")

		allCompanySlugs := data[0].Array()
		allRewarding := data[1].Array()

		allPublic := data[2].Array()

		for i := 0; i < len(allCompanySlugs); i++ {
			if !pvtOnly || (pvtOnly && !allPublic[i].Bool()) {
				if !bbpOnly || (bbpOnly && allRewarding[i].Bool()) {
					pData := GetProgramScope(token, allCompanySlugs[i].Str, categories)
					programs = append(programs, pData)
				}
			}
		}

		nb_pages = int(gjson.Get(res.BodyString, "pagination.nb_pages").Int())
		page += 1
	}

	return programs
}

func PrintAllScope(token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, delimiter string) {
	programs := GetAllProgramsScope(token, bbpOnly, pvtOnly, categories)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter, false)
	}
}
