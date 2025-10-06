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
	token       string
	urlToID     map[string]string
	handleToURL map[string]string
}

func NewPoller() *Poller {
	return &Poller{urlToID: map[string]string{}, handleToURL: map[string]string{}}
}

func (p *Poller) Name() string { return "it" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Token != "" {
		p.token = cfg.Token
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	p.urlToID = map[string]string{}
	p.handleToURL = map[string]string{}
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

			parts := strings.Split(strings.TrimSuffix(url, "/detail"), "/")
			handle := url
			if len(parts) >= 2 {
				handle = parts[len(parts)-2] + "/" + parts[len(parts)-1]
			}

			// Filtering logic from GetAllProgramsScope
			if (opts.PrivateOnly && confidentialityLevel != 4) || !opts.PrivateOnly {
				if (opts.BountyOnly && maxBounty != 0) || !opts.BountyOnly {
					p.urlToID[handle] = id
					p.handleToURL[handle] = url
					handles = append(handles, handle)
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
	pData := scope.ProgramData{Url: p.handleToURL[handle]}
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
			allowedCategories := getCategoryID(opts.Categories)
			if allowedCategories == nil || isInArray(int(categoryID), allowedCategories) {
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
	input = strings.ToLower(input)
	if input == "all" || input == "" {
		return nil
	}

	categories := map[string][]int{
		"url":      {1},
		"cidr":     {4},
		"mobile":   {2, 3},
		"android":  {2},
		"apple":    {3},
		"device":   {5},
		"other":    {6},
		"wildcard": {7},
	}
	selected, ok := categories[input]
	if !ok {
		return nil // Default to all if category is invalid
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
