package intigriti

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

type Poller struct {
	token   string
	urlToID map[string]string
}

func NewPoller(token string) *Poller { return &Poller{token: token, urlToID: map[string]string{}} }

func (p *Poller) Name() string { return "it" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Token != "" {
		p.token = cfg.Token
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	p.urlToID = map[string]string{}
	offset := 0
	limit := 500
	total := 0
	handles := []string{}

	for {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     fmt.Sprintf("https://api.intigriti.com/external/researcher/v1/programs?statusId=3&limit=%d&offset=%d", limit, offset),
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
		}, nil)

		if err != nil {
			return nil, err
		}

		if res.StatusCode == 401 {
			return nil, fmt.Errorf("invalid auth token")
		}

		body := res.BodyString
		if offset == 0 {
			total = int(gjson.Get(body, "maxCount").Int())
		}

		records := gjson.Get(body, "records").Array()
		for _, record := range records {
			id := record.Get("id").String()
			maxBounty := record.Get("maxBounty.value").Int()
			confidentialityLevel := record.Get("confidentialityLevel.id").Int()
			programPathParts := strings.Split(record.Get("webLinks.detail").String(), "=")
			if len(programPathParts) < 2 {
				continue
			}
			programPath := programPathParts[1]
			url := "https://app.intigriti.com/researcher" + programPath

			// Filtering logic from GetAllProgramsScope
			if (opts.PrivateOnly && confidentialityLevel != 4) || !opts.PrivateOnly {
				if (opts.BountyOnly && maxBounty != 0) || !opts.BountyOnly {
					p.urlToID[url] = id
					handles = append(handles, url)
				}
			}
		}

		offset += len(records)
		if offset >= total {
			break
		}
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: handle}
	id := p.urlToID[handle]
	if id == "" {
		// Ensure map is built at least once
		if _, err := p.ListProgramHandles(ctx, opts); err == nil {
			id = p.urlToID[handle]
		}
	}
	if id == "" {
		return pData, nil
	}

	res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method:  "GET",
		URL:     "https://api.intigriti.com/external/researcher/v1/programs/" + id,
		Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
	}, nil)

	if err != nil {
		return pData, err
	}

	if res.StatusCode == 401 {
		utils.Log.Fatal("Invalid auth token")
	}

	if strings.Contains(res.BodyString, "Request blocked") {
		utils.Log.Info("Rate limited. Retrying...")
		time.Sleep(2 * time.Second)
		return p.FetchProgramScope(ctx, handle, opts)
	}

	//processed := make(map[string]struct{})
	contentArray := gjson.Get(res.BodyString, "domains.content")
	contentArray.ForEach(func(key, value gjson.Result) bool {
		endpoint := value.Get("endpoint").String()
		categoryID := value.Get("type.id").Int()
		categoryValue := value.Get("type.value").Str
		tierID := value.Get("tier.id").Int()
		description := value.Get("description").Str

		if tierID != 5 { // Not out-of-scope
			if isInArray(int(categoryID), getCategoryID(opts.Categories)) {
				pData.InScope = append(pData.InScope, scope.ScopeElement{
					Target:      endpoint,
					Description: strings.ReplaceAll(description, "\n", "  "),
					Category:    categoryValue,
				})
			}
		} else {
			pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
				Target:      endpoint,
				Description: strings.ReplaceAll(description, "\n", "  "),
				Category:    categoryValue,
			})
		}
		return true
	})

	return pData, nil
}
func getCategoryID(input string) []int {
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
	selected, ok := categories[strings.ToLower(input)]
	if !ok {
		return categories["all"] // Default to all if category is invalid
	}
	return selected
}

func isInArray(val int, array []int) bool {
	for _, item := range array {
		if item == val {
			return true
		}
	}
	return false
}
