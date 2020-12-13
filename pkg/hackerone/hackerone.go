package hackerone

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"

	strip "github.com/grokify/html-strip-tags-go"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	H1_GRAPHQL_TOKEN_ENDPOINT = "https://hackerone.com/current_user/graphql_token"
	H1_GRAPHQL_ENDPOINT       = "https://hackerone.com/graphql"
	USER_AGENT                = "Mozilla/5.0 (X11; Linux x86_64; rv:82.0) Gecko/20100101 Firefox/82.0"
)

type Program struct {
	handle     string
	inScope    []string
	outOfScope []string
}

type programsData struct {
	mu      sync.Mutex
	handles []Program
}

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

func getProgramScope(graphQLToken string, handle string, bbpOnly bool, pvtOnly bool, categories []string, descToo bool) Program {
	var p Program
	p.handle = handle

	scopeQuery := `{
		"query":"query Team_assets($first_0:Int!) {query {id,...F0}} fragment F0 on Query {me {_membership_A:membership(team_handle:\"__REPLACEME__\") {permissions,id},id},_team_A:team(handle:\"__REPLACEME__\") {handle,_structured_scope_versions_A:structured_scope_versions(archived:false) {max_updated_at},_structured_scopes_B:structured_scopes(first:$first_0,archived:false,eligible_for_submission:true) {edges {node {id,asset_type,asset_identifier,rendered_instruction,max_severity,eligible_for_bounty},cursor},pageInfo {hasNextPage,hasPreviousPage}},_structured_scopes_C:structured_scopes(first:$first_0,archived:false,eligible_for_submission:false) {edges {node {id,asset_type,asset_identifier,rendered_instruction},cursor},pageInfo {hasNextPage,hasPreviousPage}},id},id}",
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
			if (bbpOnly && gjson.Get(edge.Raw, "node.eligible_for_bounty").Bool()) || !bbpOnly {
				if !descToo {
					p.inScope = append(p.inScope, gjson.Get(edge.Raw, "node.asset_identifier").Str)
				} else {
					desc := gjson.Get(edge.Raw, "node.rendered_instruction").Str
					if desc != "" {
						desc = " => " + strip.StripTags(strings.ReplaceAll(desc, "\n", " "))
					}
					p.inScope = append(p.inScope, gjson.Get(edge.Raw, "node.asset_identifier").Str+desc)
				}
			}
		}
	}
	return p
}

func getCategories(input string) []string {
	categories := map[string][]string{
		"url":      []string{"URL"},
		"cidr":     []string{"CIDR"},
		"mobile":   []string{"GOOGLE_PLAY_APP_ID", "OTHER_APK", "APPLE_STORE_APP_ID"},
		"android":  []string{"GOOGLE_PLAY_APP_ID", "OTHER_APK"},
		"apple":    []string{"APPLE_STORE_APP_ID"},
		"other":    []string{"OTHER"},
		"hardware": []string{"HARDWARE"},
		"all":      []string{"URL", "CIDR", "GOOGLE_PLAY_APP_ID", "OTHER_APK", "APPLE_STORE_APP_ID", "OTHER", "HARDWARE"},
		"code":     []string{"SOURCE_CODE"},
	}

	selectedCategory, ok := categories[strings.ToLower(input)]
	if !ok {
		log.Fatal("Invalid category")
	}
	return selectedCategory
}

// GetAllScope returns an array of programs data
func GetAllScope(graphQLToken string, bbpOnly bool, pvtOnly bool, categories string, descToo bool) []Program {

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
	var pData programsData

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

				temp := getProgramScope(graphQLToken, programHandles[handleIndex], bbpOnly, pvtOnly, getCategories(categories), descToo)
				pData.mu.Lock()
				pData.handles = append(pData.handles, temp)
				pData.mu.Unlock()

			}
			processGroup.Done()
		}()
	}

	for handleIndex := 1; handleIndex <= len(programHandles); handleIndex++ {
		handleIndices <- handleIndex
	}

	close(handleIndices)
	processGroup.Wait()

	return pData.handles

}

// PrintScope prints to stdout all the scope targets
func PrintScope(h1Token string, bbpOnly bool, pvtOnly bool, categories string, descToo bool, urlsToo bool, noToken bool) {
	graphQLToken := ""
	if !noToken {
		graphQLToken = GetGraphQLToken(h1Token)

		if graphQLToken == "----" {
			log.Fatal("Invalid __Host-session token. Use --noToken if you want public programs only")
		}
	}

	programs := GetAllScope(graphQLToken, bbpOnly, pvtOnly, categories, descToo)
	for _, program := range programs {
		for _, target := range program.inScope {
			if urlsToo {
				fmt.Println(target + " https://hackerone.com/" + program.handle)
			} else {
				fmt.Println(target)
			}
		}
	}
}

/*

GraphQL response example

{
   "data":{
      "me":{
         "id":"Z2lkOi8vaGFja2Vyb25lL1VzZXIvNTA0NTYw",
         "has_checklist_check_responses":false,
         "soft_launch_invitations":{
            "total_count":0,
            "__typename":"SoftLaunchConnection"
         },
         "__typename":"User"
      },
      "teams":{
         "pageInfo":{
            "endCursor":"ODA",
            "hasNextPage":true,
            "__typename":"PageInfo"
         },
         "edges":[
            {
               "cursor":"MQ",
               "node":{
                  "id":"Z2lkOi8vaGFja2Vyb25lL1RlYW0vMzU5Mjk=",
                  "handle":"status_im",
                  "name":"Status.im",
                  "currency":"usd",
                  "team_profile_picture":"https://profile-photos.hackerone-user-content.com/variants/000/035/929/e47e05ef1c915e815a0ce337ecde09c4457b2104_original./eb31823a4cc9f6b6bb4db930ffdf512533928a68a4255fb50a83180281a60da5",
                  "submission_state":"open",
                  "triage_active":true,
                  "state":"public_mode",
                  "started_accepting_at":"2020-10-08T16:47:19.496Z",
                  "number_of_reports_for_user":0,
                  "number_of_valid_reports_for_user":0,
                  "bounty_earned_for_user":0.0,
                  "last_invitation_accepted_at_for_user":"2020-05-13T22:46:04.563Z",
                  "bookmarked":false,
                  "external_program":null,
                  "__typename":"Team",
                  "average_bounty_lower_amount":250,
                  "average_bounty_upper_amount":500,
                  "base_bounty":50,
                  "resolved_report_count":27
               },
               "__typename":"TeamEdge"
            },
            {
               "cursor":"Mg",
               "node":{
                  "id":"Z2lkOi8vaGFja2Vyb25lL1RlYW0vMzI5NzI=",
                  "handle":"logitech",
                  "name":"Logitech",
                  "currency":"usd",
                  "team_profile_picture":"https://profile-photos.hackerone-user-content.com/variants/000/032/972/8aa1ae9384c034f209edabfd44bac468c0bbbcdb_original./eb31823a4cc9f6b6bb4db930ffdf512533928a68a4255fb50a83180281a60da5",
                  "submission_state":"open",
                  "triage_active":true,
                  "state":"public_mode",
                  "started_accepting_at":"2020-10-07T15:30:31.789Z",
                  "number_of_reports_for_user":1,
                  "number_of_valid_reports_for_user":1,
                  "bounty_earned_for_user":0.0,
                  "last_invitation_accepted_at_for_user":"2020-09-19T08:57:01.221Z",
                  "bookmarked":false,
                  "external_program":null,
                  "__typename":"Team",
                  "average_bounty_lower_amount":200,
                  "average_bounty_upper_amount":200,
                  "base_bounty":100,
                  "resolved_report_count":269
               },
               "__typename":"TeamEdge"
            },
*/
