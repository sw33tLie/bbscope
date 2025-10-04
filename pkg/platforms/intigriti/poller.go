package intigriti

import (
	"context"
	"fmt"
	"strings"

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
	// Page once; fill url->id map; return URLs as handles
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
		body := string(res.BodyString)
		if offset == 0 {
			total = int(gjson.Get(body, "maxCount").Int())
		}
		records := gjson.Get(body, "records").Array()
		for _, record := range records {
			id := record.Get("id").String()
			programPath := strings.Split(record.Get("webLinks.detail").String(), "=")[1]
			url := "https://app.intigriti.com/researcher" + programPath
			p.urlToID[url] = id
			handles = append(handles, url)
		}
		offset += len(records)
		if offset >= total {
			break
		}
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	id := p.urlToID[handle]
	if id == "" {
		// Ensure map is built at least once
		if _, err := p.ListProgramHandles(ctx, opts); err == nil {
			id = p.urlToID[handle]
		}
	}
	if id == "" {
		return scope.ProgramData{Url: handle}, nil
	}
	pd := GetProgramScope(p.token, id, "all", false, false)
	pd.Url = handle
	return pd, nil
}
