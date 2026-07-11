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

	// Fetch program details for metadata. This is a single additional call;
	// failure is non-fatal — scope collection proceeds without metadata.
	progRes, progErr := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method:  "GET",
		URL:     "https://api.hackerone.com/v1/hackers/programs/" + handle,
		Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + p.authB64}},
	}, nil)
	if progErr == nil && progRes.StatusCode == 200 {
		pData.Metadata = extractH1Metadata(progRes.BodyString)
	}

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
		}

		if opts.BountyOnly && len(pData.InScope) == 0 {
			pData.OutOfScope = []scope.ScopeElement{}
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

// extractH1Metadata parses the body of GET /v1/hackers/programs/{handle} into a
// ProgramMetadata. HackerOne's public API does not expose reward grids,
// qualifying vulnerabilities, or testing instructions, so those fields remain nil.
func extractH1Metadata(body string) *scope.ProgramMetadata {
	handle := gjson.Get(body, "attributes.handle").Str
	tagline := gjson.Get(body, "attributes.tagline").Str
	offersBounties := gjson.Get(body, "attributes.offers_bounties").Bool()
	state := gjson.Get(body, "attributes.state").Str
	submissionState := gjson.Get(body, "attributes.submission_state").Str

	title := handle
	if tagline != "" {
		title = tagline + " (" + handle + ")"
	}

	md := &scope.ProgramMetadata{
		Title:      title,
		Tagline:    tagline,
		IsPublic:   boolPtr(state != "soft_launched"),
		IsBounty:   boolPtr(offersBounties),
		IsDisabled: boolPtr(submissionState == "paused" || submissionState == "closed"),
	}

	if !offersBounties && submissionState == "open" {
		md.IsVDP = boolPtr(true)
		md.ProgramType = "vdp"
	} else {
		md.ProgramType = "bug-bounty"
	}

	return md
}

// boolPtr returns a pointer to the given bool. scope.boolPtr is unexported, so
// the hackerone package keeps a local copy.
func boolPtr(v bool) *bool { return &v }
