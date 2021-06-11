package hackerone

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/sw33tLie/bbscope/internal/scope"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	H1_GRAPHQL_TOKEN_ENDPOINT = "https://hackerone.com/current_user/graphql_token"
	H1_GRAPHQL_ENDPOINT       = "https://hackerone.com/graphql"
	USER_AGENT                = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
)

// GetGraphQLToken gets a GraphQL token by using the session cookie
func GetGraphQLToken(cookie string) string {
	req, err := http.NewRequest("GET", H1_GRAPHQL_TOKEN_ENDPOINT, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Cookie", "__Host-session="+cookie)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	return string(gjson.Get(string(body), "graphql_token").Str)
}

func getProgramScope(graphQLToken string, handle string, bbpOnly bool, pvtOnly bool, categories []string) (pData scope.ProgramData) {
	pData.Url = "https://hackerone.com/" + handle

	scopeQuery := `{
		"query":"query Team_assets($first_0:Int!) {query {id,...F0}} fragment F0 on Query {me {_membership_A:membership(team_handle:\"__REPLACEME__\") {permissions,id},id},_team_A:team(handle:\"__REPLACEME__\") {handle,_structured_scope_versions_A:structured_scope_versions(archived:false) {max_updated_at},_structured_scopes_B:structured_scopes(first:$first_0,archived:false,eligible_for_submission:true) {edges {node {id,asset_type,asset_identifier,instruction,max_severity,eligible_for_bounty},cursor},pageInfo {hasNextPage,hasPreviousPage}},_structured_scopes_C:structured_scopes(first:$first_0,archived:false,eligible_for_submission:false) {edges {node {id,asset_type,asset_identifier,instruction},cursor},pageInfo {hasNextPage,hasPreviousPage}},id},id}",
		"variables":{
		   "first_0":500
		}
	 }`

	scopeQuery = strings.ReplaceAll(scopeQuery, "__REPLACEME__", handle)

	req, err := http.NewRequest("POST", H1_GRAPHQL_ENDPOINT, bytes.NewBuffer([]byte(scopeQuery)))
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Auth-Token", graphQLToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	for _, edge := range gjson.Get(string(body), "data.query._team_A._structured_scopes_B.edges").Array() {
		catFound := false
		assetCategory := gjson.Get(edge.Raw, "node.asset_type").Str
		for _, cat := range categories {
			if cat == assetCategory {
				catFound = true
				break
			}
		}
		if catFound {
			if !bbpOnly || (bbpOnly && gjson.Get(edge.Raw, "node.eligible_for_bounty").Bool()) {
				pData.InScope = append(pData.InScope, scope.ScopeElement{
					Target:      gjson.Get(edge.Raw, "node.asset_identifier").Str,
					Description: strings.ReplaceAll(gjson.Get(edge.Raw, "node.instruction").Str, "\n", "  "),
					Category:    "", // TODO
				})

				/*
					if !descToo {
						p.inScope = append(p.inScope)
					} else {
						p.inScope = append(p.inScope, gjson.Get(edge.Raw, "node.asset_identifier").Str+desc)
					}*/
			}
		}
	}

	if len(pData.InScope) == 0 {
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

func getProgramHandles(graphQLToken string, pvtOnly bool) []string {
	getProgramsQuery := `
	{
		"operationName":"MyProgramsQuery",
		"variables":{
		   "where":{
			  "_and":[
				 {
					"_or":[
					   {
						  "submission_state":{
							 "_eq":"open"
						  }
					   },
					   {
						  "submission_state":{
							 "_eq":"api_only"
						  }
					   }
					]
				 }
			  ]
		   },
		   "count":100,
		   "orderBy":null,
		   "secureOrderBy":{
			  "started_accepting_at":{
				 "_direction":"DESC"
			  }
		   },
		   "cursor":""
		},
		"query":"query MyProgramsQuery($cursor: String, $count: Int, $where: FiltersTeamFilterInput, $orderBy: TeamOrderInput, $secureOrderBy: FiltersTeamFilterOrder) {\n  me {\n    id\n    ...MyHackerOneSubHeader\n    __typename\n  }\n  teams(first: $count, after: $cursor, order_by: $orderBy, secure_order_by: $secureOrderBy, where: $where) {\n    pageInfo {\n      endCursor\n      hasNextPage\n      __typename\n    }\n    edges {\n      cursor\n      node {\n        id\n        handle\n        name\n        currency\n        team_profile_picture: profile_picture(size: medium)\n        submission_state\n        triage_active\n        state\n        started_accepting_at\n        number_of_reports_for_user\n        number_of_valid_reports_for_user\n        bounty_earned_for_user\n        last_invitation_accepted_at_for_user\n        bookmarked\n        external_program {\n          id\n          __typename\n        }\n        ...TeamLinkWithMiniProfile\n        ...TeamTableAverageBounty\n        ...TeamTableMinimumBounty\n        ...TeamTableResolvedReports\n        __typename\n      }\n      __typename\n    }\n    __typename\n  }\n}\n\nfragment TeamLinkWithMiniProfile on Team {\n  id\n  handle\n  name\n  __typename\n}\n\nfragment TeamTableAverageBounty on Team {\n  id\n  currency\n  average_bounty_lower_amount\n  average_bounty_upper_amount\n  __typename\n}\n\nfragment TeamTableMinimumBounty on Team {\n  id\n  currency\n  base_bounty\n  __typename\n}\n\nfragment TeamTableResolvedReports on Team {\n  id\n  resolved_report_count\n  __typename\n}\n\nfragment MyHackerOneSubHeader on User {\n  id\n  has_checklist_check_responses\n  soft_launch_invitations(state: open) {\n    total_count\n    __typename\n  }\n  __typename\n}\n"
	 }`

	lastCursor := ""
	var programHandles []string

	for {
		currentProgramsQuery, _ := sjson.Set(getProgramsQuery, "variables.cursor", lastCursor)
		req, err := http.NewRequest("POST", H1_GRAPHQL_ENDPOINT, bytes.NewBuffer([]byte(currentProgramsQuery)))
		if err != nil {
			log.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", USER_AGENT)
		req.Header.Set("X-Auth-Token", graphQLToken)

		client := &http.Client{}

		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		body, _ := ioutil.ReadAll(resp.Body)

		if len(gjson.Get(string(body), "data.teams.edges").Array()) == 0 {
			break
		}

		for _, edge := range gjson.Get(string(body), "data.teams.edges").Array() {
			lastCursor = gjson.Get(edge.Raw, "cursor").Str
			if !pvtOnly || (pvtOnly && gjson.Get(edge.Raw, "node.state").Str == "soft_launched") {
				programHandles = append(programHandles, gjson.Get(edge.Raw, "node.handle").Str)
			}
		}
	}

	return programHandles
}

// GetAllProgramsScope xxx
func GetAllProgramsScope(graphQLToken string, bbpOnly bool, pvtOnly bool, categories string) (programs []scope.ProgramData) {
	programHandles := getProgramHandles(graphQLToken, pvtOnly)
	threads := 50
	handleIndices := make(chan int, threads)
	processGroup := new(sync.WaitGroup)
	processGroup.Add(threads)

	for i := 0; i < threads; i++ {
		go func() {
			for {
				handleIndex := <-handleIndices - 1

				if handleIndex == -1 {
					break
				}

				programs = append(programs, getProgramScope(graphQLToken, programHandles[handleIndex], bbpOnly, pvtOnly, getCategories(categories)))
			}
			processGroup.Done()
		}()
	}

	for handleIndex := 1; handleIndex <= len(programHandles); handleIndex++ {
		handleIndices <- handleIndex
	}

	close(handleIndices)
	processGroup.Wait()

	return programs
}

// PrintAllScope prints to stdout all scope elements of all targets
func PrintAllScope(h1Token string, bbpOnly bool, pvtOnly bool, categories string, outputFlags string, delimiter string, noToken bool) {
	graphQLToken := ""
	if !noToken {
		graphQLToken = GetGraphQLToken(h1Token)

		if graphQLToken == "----" {
			log.Fatal("Invalid __Host-session token. Use --noToken if you want public programs only")
		}
	}

	programs := GetAllProgramsScope(graphQLToken, bbpOnly, pvtOnly, categories)
	for _, pData := range programs {
		scope.PrintProgramScope(pData, outputFlags, delimiter)
	}
}

/*
func ListPrograms(h1Token string, bbpOnly bool, pvtOnly bool, categories string, noToken bool) {
	graphQLToken := ""
	if !noToken {
		graphQLToken = GetGraphQLToken(h1Token)

		if graphQLToken == "----" {
			log.Fatal("Invalid __Host-session token. Use --noToken if you want public programs only")
		}
	}

	if !bbpOnly && categories == "all" {
		// If we don't want BBPs or custom categories, we can do it faster
		programHandles := getProgramHandles(graphQLToken, pvtOnly)
		for _, handle := range programHandles {
			fmt.Println("https://hackerone.com/" + handle)
		}
	} else {
		programs := GetAllProgramsScope(graphQLToken, bbpOnly, pvtOnly, categories, false)
		for _, program := range programs {
			fmt.Println("https://hackerone.com/" + program.handle)
		}
	}
}*/
