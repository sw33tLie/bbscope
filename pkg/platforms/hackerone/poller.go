package hackerone

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

// Poller adapts existing H1 code to the generic PlatformPoller interface.
type Poller struct {
	authB64 string
}

// NewPoller builds a HackerOne poller from username and API token.
func NewPoller(username, token string) *Poller {
	raw := username + ":" + token
	return &Poller{authB64: base64.StdEncoding.EncodeToString([]byte(raw))}
}

func (p *Poller) Name() string { return "h1" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Username != "" && cfg.Token != "" {
		raw := cfg.Username + ":" + cfg.Token
		p.authB64 = base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	var handles []string
	currentURL := "https://api.hackerone.com/v1/hackers/programs?page%5Bsize%5D=100"
	for {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     currentURL,
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + p.authB64}},
		}, nil)

		if err != nil {
			utils.Log.Warn("HTTP request failed, retrying: ", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if res.StatusCode != 200 {
			return nil, fmt.Errorf("fetching failed. Got status Code: %d", res.StatusCode)
		}

		for i := 0; i < int(gjson.Get(res.BodyString, "data.#").Int()); i++ {
			handle := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.handle").Str
			state := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.state").Str
			submissionState := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.submission_state").Str
			offersBounties := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.offers_bounties").Bool()

			// Private programs have state "soft_launched"
			isPrivate := state == "soft_launched"

			if submissionState != "open" {
				continue // Skip inactive programs
			}

			if opts.PrivateOnly && !isPrivate {
				continue
			}

			if opts.BountyOnly && !offersBounties {
				continue
			}

			handles = append(handles, handle)
		}

		currentURL = gjson.Get(res.BodyString, "links.next").Str
		if currentURL == "" {
			break
		}
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: "https://hackerone.com/" + handle}
	currentPageURL := "https://api.hackerone.com/v1/hackers/programs/" + handle + "/structured_scopes?page%5Bnumber%5D=1&page%5Bsize%5D=100"
	categoryStrings := scope.GetAllStringsForCategories(opts.Categories)

	for {
		var res *whttp.WHTTPRes
		var err error
		retries := 3
		var statusCode int

		for retries > 0 {
			res, err = whttp.SendHTTPRequest(&whttp.WHTTPReq{
				Method:  "GET",
				URL:     currentPageURL,
				Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + p.authB64}},
			}, nil)

			if err == nil && strings.Contains(res.BodyString, "\"data\":") {
				statusCode = res.StatusCode
				break
			}
			retries--
			time.Sleep(2 * time.Second)
		}

		if retries == 0 {
			return scope.ProgramData{}, fmt.Errorf("failed to retrieve data for %s after 3 attempts with status %d", handle, statusCode)
		}

		assetCount := int(gjson.Get(res.BodyString, "data.#").Int())
		isDumpAll := categoryStrings == nil

		for i := 0; i < assetCount; i++ {
			assetCategory := strings.ToLower(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_type").Str)
			catFound := isDumpAll
			if !isDumpAll {
				for _, cat := range categoryStrings {
					if cat == assetCategory {
						catFound = true
						break
					}
				}
			}

			if catFound {
				eligibleForBounty := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_bounty").Bool()
				eligibleForSubmission := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.eligible_for_submission").Bool()
				instruction := strings.ReplaceAll(gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.instruction").Str, "\n", "  ")
				target := gjson.Get(res.BodyString, "data."+strconv.Itoa(i)+".attributes.asset_identifier").Str

				if eligibleForSubmission {
					if !opts.BountyOnly || eligibleForBounty {
						pData.InScope = append(pData.InScope, scope.ScopeElement{
							Target:      target,
							Description: instruction,
							Category:    assetCategory,
						})
					}
				} else {
					pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
						Target:      target,
						Description: instruction,
						Category:    assetCategory,
					})
				}
			}
		}

		if opts.BountyOnly && len(pData.InScope) == 0 {
			pData.OutOfScope = []scope.ScopeElement{}
		}

		if assetCount == 0 {
			pData.InScope = append(pData.InScope, scope.ScopeElement{Target: "NO_IN_SCOPE_TABLE"})
		}

		nextPageURL := gjson.Get(res.BodyString, "links.next").Str
		if nextPageURL != "" {
			currentPageURL = nextPageURL
		} else {
			break
		}
	}
	return pData, nil
}
