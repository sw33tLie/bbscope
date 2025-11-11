package immunefi

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

type Poller struct{}

func (p *Poller) Name() string { return "immunefi" }

// Authenticate is a no-op for Immunefi (no auth required)
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    PLATFORM_URL + "/bug-bounty/",
			Headers: []whttp.WHTTPHeader{
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))
	if err != nil {
		return nil, err
	}

	var programURLs []string
	doc.Find("#__NEXT_DATA__").Each(func(index int, s *goquery.Selection) {
		json := s.Contents().Text()
		jsonPrograms := gjson.Get(json, "props.pageProps.bounties")

		for _, program := range jsonPrograms.Array() {
			programID := gjson.Get(program.Raw, "id")
			isExternal := gjson.Get(program.Raw, "is_external").Bool()

			if !isExternal {
				programURLs = append(programURLs, PLATFORM_URL+"/bug-bounty/"+programID.Str+"/information/")
			}
		}
	})

	return programURLs, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: handle}

	res, err := whttp.SendHTTPRequest(
		&whttp.WHTTPReq{
			Method: "GET",
			URL:    handle,
			Headers: []whttp.WHTTPHeader{
				{Name: "Accept", Value: "*/*"},
			},
		}, nil)

	if err != nil {
		return pData, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.BodyString))
	if err != nil {
		return pData, err
	}

	selectedCategories := getCategories(opts.Categories)

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

		pData.InScope = tempScope
		pData.OutOfScope = nil
	})

	return pData, nil
}
