package immunefi

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

const maxRetries = 20

type Poller struct{}

func (p *Poller) Name() string { return "immunefi" }

// Authenticate is a no-op for Immunefi (no auth required)
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

// fetchWithRetry sends an HTTP request with retry logic for 429 rate limits.
// It will retry up to maxRetries times with exponential backoff.
func fetchWithRetry(url string) (*whttp.WHTTPRes, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		res, err := whttp.SendHTTPRequest(
			&whttp.WHTTPReq{
				Method: "GET",
				URL:    url,
				Headers: []whttp.WHTTPHeader{
					{Name: "Accept", Value: "*/*"},
					{Name: "Rsc", Value: "1"},
				},
			}, nil)

		if err != nil {
			lastErr = err
			// Network error, retry with backoff
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		if res.StatusCode == 429 {
			// Rate limited, wait with exponential backoff and retry
			backoff := time.Duration(attempt+1) * 2 * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
			continue
		}

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			return res, nil
		}

		// Other error status, return it
		lastErr = fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("failed after %d retries", maxRetries)
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	res, err := fetchWithRetry(PLATFORM_URL + "/bug-bounty/")
	if err != nil {
		return nil, err
	}

	// The RSC response contains embedded JSON. Extract the bounties array.
	// Look for "bounties":[...] pattern
	bountiesRegex := regexp.MustCompile(`"bounties":\[`)
	match := bountiesRegex.FindStringIndex(res.BodyString)
	if match == nil {
		return nil, nil
	}

	// Find the matching closing bracket for the bounties array
	startIdx := match[0] + len(`"bounties":`)
	bountyJSON := extractJSONArray(res.BodyString[startIdx:])
	if bountyJSON == "" {
		return nil, nil
	}

	var programURLs []string
	jsonPrograms := gjson.Parse(bountyJSON)

	for _, program := range jsonPrograms.Array() {
		programID := gjson.Get(program.Raw, "id").String()
		inviteOnly := gjson.Get(program.Raw, "inviteOnly").Bool()

		if programID != "" && !inviteOnly {
			programURLs = append(programURLs, PLATFORM_URL+"/bug-bounty/"+programID+"/information/")
		}
	}

	return programURLs, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	pData := scope.ProgramData{Url: handle}

	res, err := fetchWithRetry(handle)
	if err != nil {
		return pData, err
	}

	selectedCategories := getCategories(opts.Categories)

	// Extract assets array from RSC response
	assetsRegex := regexp.MustCompile(`"assets":\[`)
	match := assetsRegex.FindStringIndex(res.BodyString)
	if match == nil {
		return pData, nil
	}

	startIdx := match[0] + len(`"assets":`)
	assetsJSON := extractJSONArray(res.BodyString[startIdx:])
	if assetsJSON == "" {
		return pData, nil
	}

	var tempScope []scope.ScopeElement
	jsonAssets := gjson.Parse(assetsJSON)

	for _, asset := range jsonAssets.Array() {
		elementTarget := gjson.Get(asset.Raw, "url").String()
		elementType := gjson.Get(asset.Raw, "type").String()
		elementDesc := gjson.Get(asset.Raw, "description").String()

		for _, currentCat := range selectedCategories {
			if currentCat == "websites_and_applications" && strings.Contains(elementType, "websites_and_applications") {
				tempScope = append(tempScope, scope.ScopeElement{
					Target:      elementTarget,
					Description: elementDesc,
					Category:    currentCat,
				})
			} else if currentCat == "smart_contract" && strings.Contains(elementType, "smart_contract") {
				tempScope = append(tempScope, scope.ScopeElement{
					Target:      elementTarget,
					Description: elementDesc,
					Category:    currentCat,
				})
			}
		}
	}

	pData.InScope = tempScope
	pData.OutOfScope = nil

	return pData, nil
}

// extractJSONArray extracts a JSON array starting at position 0 of the input string.
// It handles nested brackets and returns the complete array including brackets.
func extractJSONArray(s string) string {
	if len(s) == 0 || s[0] != '[' {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return ""
}
