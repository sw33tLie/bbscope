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

		if res.StatusCode == 400 && strings.Contains(res.BodyString, "\"Invalid Parameter\"") {
			break // API pagination limit reached, return what we have
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
	categoryStrings := scope.GetAllStringsForCategories(opts.Categories)

	// Use filter[id__gt] cursor-based pagination instead of page[number]
	// to avoid HackerOne's 9,999 record pagination limit.
	// See: https://api.hackerone.com/hacker-resources/ — structured_scopes endpoint docs.
	lastID := "0"

	for {
		currentPageURL := fmt.Sprintf(
			"https://api.hackerone.com/v1/hackers/programs/%s/structured_scopes?page%%5Bsize%%5D=100&filter%%5Bid__gt%%5D=%s",
			handle, lastID,
		)

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
			// Don't retry on non-retryable HTTP errors
			if err == nil && res != nil && (res.StatusCode == 400 || res.StatusCode == 403 || res.StatusCode == 404) {
				statusCode = res.StatusCode
				break
			}
			retries--
			time.Sleep(2 * time.Second)
		}

		if retries == 0 || (res != nil && !strings.Contains(res.BodyString, "\"data\":")) {
			// Return whatever we've collected so far instead of failing
			if len(pData.InScope) > 0 || len(pData.OutOfScope) > 0 {
				utils.Log.Warnf("Partial data for %s: stopped at status %d", handle, statusCode)
				return pData, nil
			}
			return pData, fmt.Errorf("failed to retrieve data for %s after 3 attempts with status %d", handle, statusCode)
		}

		assetCount := int(gjson.Get(res.BodyString, "data.#").Int())
		if assetCount == 0 {
			break // No more results
		}

		isDumpAll := categoryStrings == nil

		for i := 0; i < assetCount; i++ {
			prefix := "data." + strconv.Itoa(i)
			assetCategory := strings.ToLower(gjson.Get(res.BodyString, prefix+".attributes.asset_type").Str)
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
				eligibleForBounty := gjson.Get(res.BodyString, prefix+".attributes.eligible_for_bounty").Bool()
				eligibleForSubmission := gjson.Get(res.BodyString, prefix+".attributes.eligible_for_submission").Bool()
				instruction := strings.ReplaceAll(gjson.Get(res.BodyString, prefix+".attributes.instruction").Str, "\n", "  ")
				target := gjson.Get(res.BodyString, prefix+".attributes.asset_identifier").Str

				if eligibleForSubmission {
					if !opts.BountyOnly || eligibleForBounty {
						pData.InScope = append(pData.InScope, scope.ScopeElement{
							Target:      target,
							Description: instruction,
							Category:    assetCategory,
							IsBBP:       eligibleForBounty,
						})
					}
				} else {
					pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
						Target:      target,
						Description: instruction,
						Category:    assetCategory,
						IsBBP:       eligibleForBounty,
					})
				}
			}

			// Track the last ID for cursor-based pagination
			itemID := gjson.Get(res.BodyString, prefix+".id").Str
			if itemID != "" {
				lastID = itemID
			}
		}

		if opts.BountyOnly && len(pData.InScope) == 0 {
			pData.OutOfScope = []scope.ScopeElement{}
		}

		// If we got fewer results than page size, we're done
		if assetCount < 100 {
			break
		}
	}
	return pData, nil
}