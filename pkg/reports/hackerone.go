package reports

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

// H1Fetcher fetches reports from the HackerOne Hacker API.
type H1Fetcher struct {
	authB64 string
}

// NewH1Fetcher creates a fetcher using the same base64 auth pattern as the poller.
func NewH1Fetcher(username, token string) *H1Fetcher {
	raw := username + ":" + token
	return &H1Fetcher{authB64: base64.StdEncoding.EncodeToString([]byte(raw))}
}

// ListReports fetches all report summaries matching the given options.
func (f *H1Fetcher) ListReports(ctx context.Context, opts FetchOptions) ([]ReportSummary, error) {
	var summaries []ReportSummary

	queryFilter := buildQueryFilter(opts)
	currentURL := "https://api.hackerone.com/v1/hackers/me/reports?page%5Bsize%5D=100"
	if queryFilter != "" {
		currentURL += "&filter%5Bkeyword%5D=" + queryFilter
	}

	for {
		body, err := f.doRequest(currentURL)
		if err != nil {
			return summaries, err
		}
		if body == "" {
			break // non-retryable status, stop
		}

		count := int(gjson.Get(body, "data.#").Int())
		for i := 0; i < count; i++ {
			prefix := "data." + strconv.Itoa(i)
			summary := ReportSummary{
				ID:             gjson.Get(body, prefix+".id").String(),
				Title:          gjson.Get(body, prefix+".attributes.title").String(),
				State:          gjson.Get(body, prefix+".attributes.state").String(),
				Substate:       gjson.Get(body, prefix+".attributes.substate").String(),
				CreatedAt:      gjson.Get(body, prefix+".attributes.created_at").String(),
				SeverityRating: gjson.Get(body, prefix+".relationships.severity.data.attributes.rating").String(),
			}

			// Program handle from relationships
			summary.ProgramHandle = gjson.Get(body, prefix+".relationships.program.data.attributes.handle").String()

			summaries = append(summaries, summary)
		}

		nextURL := gjson.Get(body, "links.next").String()
		if nextURL == "" {
			break
		}
		currentURL = nextURL
	}

	return summaries, nil
}

// FetchReport fetches the full detail of a single report by ID.
func (f *H1Fetcher) FetchReport(ctx context.Context, reportID string) (*Report, error) {
	url := "https://api.hackerone.com/v1/hackers/reports/" + reportID

	body, err := f.doRequest(url)
	if err != nil {
		return nil, err
	}
	if body == "" {
		return nil, fmt.Errorf("report %s: not found or not accessible", reportID)
	}

	r := &Report{
		ID:                       gjson.Get(body, "data.id").String(),
		Title:                    gjson.Get(body, "data.attributes.title").String(),
		State:                    gjson.Get(body, "data.attributes.state").String(),
		Substate:                 gjson.Get(body, "data.attributes.substate").String(),
		CreatedAt:                formatTimestamp(gjson.Get(body, "data.attributes.created_at").String()),
		TriagedAt:                formatTimestamp(gjson.Get(body, "data.attributes.triaged_at").String()),
		ClosedAt:                 formatTimestamp(gjson.Get(body, "data.attributes.closed_at").String()),
		DisclosedAt:              formatTimestamp(gjson.Get(body, "data.attributes.disclosed_at").String()),
		VulnerabilityInformation: gjson.Get(body, "data.attributes.vulnerability_information").String(),
		Impact:                   gjson.Get(body, "data.attributes.impact").String(),
		ProgramHandle:            gjson.Get(body, "data.relationships.program.data.attributes.handle").String(),
		SeverityRating:           gjson.Get(body, "data.relationships.severity.data.attributes.rating").String(),
		CVSSScore:                gjson.Get(body, "data.relationships.severity.data.attributes.score").String(),
		WeaknessName:             gjson.Get(body, "data.relationships.weakness.data.attributes.name").String(),
		WeaknessCWE:              gjson.Get(body, "data.relationships.weakness.data.attributes.external_id").String(),
		AssetIdentifier:          gjson.Get(body, "data.relationships.structured_scope.data.attributes.asset_identifier").String(),
	}

	// Bounty amounts — sum all bounty relationships
	var totalBounty float64
	bounties := gjson.Get(body, "data.relationships.bounties.data")
	if bounties.Exists() {
		bounties.ForEach(func(_, v gjson.Result) bool {
			totalBounty += v.Get("attributes.amount").Float()
			return true
		})
	}
	if totalBounty > 0 {
		r.BountyAmount = fmt.Sprintf("%.2f", totalBounty)
	}

	// CVE IDs
	cves := gjson.Get(body, "data.attributes.cve_ids")
	if cves.Exists() {
		cves.ForEach(func(_, v gjson.Result) bool {
			if id := v.String(); id != "" {
				r.CVEIDs = append(r.CVEIDs, id)
			}
			return true
		})
	}

	return r, nil
}

// doRequest performs an authenticated GET with retries and rate-limit handling.
func (f *H1Fetcher) doRequest(url string) (string, error) {
	retries := 3
	for retries > 0 {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     url,
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Basic " + f.authB64}},
		}, nil)

		if err != nil {
			retries--
			utils.Log.Warnf("HTTP request failed (%s), retrying: %v", url, err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Rate limited
		if res.StatusCode == 429 {
			utils.Log.Warn("Rate limited by HackerOne, waiting 60s...")
			time.Sleep(60 * time.Second)
			continue // don't decrement retries for rate limits
		}

		// Non-retryable errors
		if res.StatusCode == 400 || res.StatusCode == 403 || res.StatusCode == 404 {
			utils.Log.Warnf("Got status %d for %s, skipping", res.StatusCode, url)
			return "", nil
		}

		if res.StatusCode != 200 {
			retries--
			utils.Log.Warnf("Got status %d for %s, retrying", res.StatusCode, url)
			time.Sleep(2 * time.Second)
			continue
		}

		return res.BodyString, nil
	}

	return "", fmt.Errorf("failed to fetch %s after retries", url)
}

// buildQueryFilter builds a Lucene-syntax filter string for the H1 API.
func buildQueryFilter(opts FetchOptions) string {
	var parts []string

	if len(opts.Programs) > 0 {
		for _, p := range opts.Programs {
			parts = append(parts, "team:"+p)
		}
	}

	if len(opts.States) > 0 {
		for _, s := range opts.States {
			parts = append(parts, "substate:"+s)
		}
	}

	if len(opts.Severities) > 0 {
		for _, s := range opts.Severities {
			parts = append(parts, "severity_rating:"+s)
		}
	}

	return strings.Join(parts, " ")
}

// formatTimestamp converts an ISO 8601 timestamp to a human-readable format.
func formatTimestamp(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts // return as-is if unparseable
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
