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

// intigritiTypeIDToLabel maps the Intigriti asset typeId (as used by the
// assetsCollection API shape) to a human-readable category label, mirroring
// the strings returned by the legacy domains.content `type.value` field.
var intigritiTypeIDToLabel = map[int64]string{
	1: "URL",
	2: "Android",
	3: "Apple",
	4: "CIDR",
	5: "Device",
	6: "Other",
	7: "Wildcard",
}

// bountyInfo holds reward range info from the listing endpoint, since the
// detail endpoint does not include bounty amounts.
type bountyInfo struct {
	minBounty int
	maxBounty int
	currency  string
	industry  string
}

type Poller struct {
	token          string
	urlToID        map[string]string
	handleToURL    map[string]string
	handleToBounty map[string]bountyInfo
}

func NewPoller() *Poller {
	return &Poller{urlToID: map[string]string{}, handleToURL: map[string]string{}, handleToBounty: map[string]bountyInfo{}}
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
	p.handleToBounty = map[string]bountyInfo{}
	offset := 0
	limit := 500
	total := 0
	handles := []string{}

	for {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     fmt.Sprintf("https://api.intigriti.com/external/researcher/v1/programs?limit=%d&offset=%d", limit, offset),
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
			// Only keep Open (3) and Suspended (4) programs.
			// Suspended programs are temporarily paused (near budget limit) but still active.
			statusID := record.Get("status.id").Int()
			if statusID != 3 && statusID != 4 {
				continue
			}

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
					p.handleToBounty[handle] = bountyInfo{
						minBounty: int(record.Get("minBounty.value").Int()),
						maxBounty: int(maxBounty),
						currency:  record.Get("minBounty.currency").String(),
						industry:  record.Get("industry").String(),
					}
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

	// First pass: collect targets and determine if program is BBP.
	// A program is BBP if at least one in-scope target has a tier other than "No Bounty".
	type target struct {
		endpoint    string
		category    string
		description string
		inScope     bool
		assetValue  string
	}
	var targets []target
	isBBP := false

	contentArray := gjson.Get(res.BodyString, "domains.content")
	contentArray.ForEach(func(key, value gjson.Result) bool {
		endpoint := value.Get("endpoint").String()
		categoryID := value.Get("type.id").Int()
		categoryValue := value.Get("type.value").Str
		tierID := value.Get("tier.id").Int()
		tierValue := value.Get("tier.value").Str
		description := value.Get("description").Str

		if tierID != 5 { // Not out-of-scope
			allowedCategories := getCategoryID(opts.Categories)
			if allowedCategories == nil || isInArray(int(categoryID), allowedCategories) {
				targets = append(targets, target{
					endpoint:    endpoint,
					category:    categoryValue,
					description: strings.ReplaceAll(description, "\n", "  "),
					inScope:     true,
					assetValue:  intigritiAssetValueFromTierID(tierID),
				})
				if tierValue != "No Bounty" {
					isBBP = true
				}
			}
		} else {
			targets = append(targets, target{
				endpoint:    endpoint,
				category:    categoryValue,
				description: strings.ReplaceAll(description, "\n", "  "),
				inScope:     false,
				assetValue:  intigritiAssetValueFromTierID(tierID),
			})
		}
		return true
	})

	// Fallback to the newer assetsCollection shape when the legacy
	// domains.content array is empty or absent. Each asset entry carries a
	// bountyTierId: 5 (or missing) means out-of-scope, anything else is
	// in-scope. We treat any in-scope asset as a bounty-bearing target for the
	// purposes of IsBBP, since the assetsCollection tiers are bounty tiers.
	if len(targets) == 0 {
		latest := latestVersionEntry(res.BodyString, "assetsCollection")
		assets := latest.Get("content.assetsAndGroups").Array()
		for _, asset := range assets {
			endpoint := asset.Get("name").String()
			typeID := asset.Get("typeId").Int()
			category := intigritiTypeIDToLabel[typeID]
			if category == "" {
				category = fmt.Sprintf("type-%d", typeID)
			}
			description := asset.Get("description").String()

			tier := asset.Get("bountyTierId")
			inScope := false
			if tier.Exists() && tier.Int() != 5 {
				inScope = true
				isBBP = true
			}

			targets = append(targets, target{
				endpoint:    endpoint,
				category:    category,
				description: strings.ReplaceAll(description, "\n", "  "),
				inScope:     inScope,
				assetValue:  intigritiAssetValueFromBountyTierID(tier),
			})
		}
	}

	// Second pass: build ScopeElements with IsBBP set
	for _, t := range targets {
		elem := scope.ScopeElement{
			Target:      t.endpoint,
			Description: t.description,
			Category:    t.category,
			IsBBP:       isBBP,
			AssetValue:  t.assetValue,
		}
		if t.inScope {
			pData.InScope = append(pData.InScope, elem)
		} else {
			pData.OutOfScope = append(pData.OutOfScope, elem)
		}
	}

	pData.Metadata = extractIntigritiMetadata(res.BodyString, isBBP, p.handleToBounty[handle])

	return pData, nil
}

// intigritiAssetValueFromTierID maps an Intigriti domains.content tier.id to
// a normalized asset value string. tier.id >= 3 are primary assets (high),
// 2 is secondary (medium), 1 is the lowest / No Bounty tier (low). A missing
// tier yields an empty string.
func intigritiAssetValueFromTierID(tierID int64) string {
	switch {
	case tierID == 0:
		return ""
	case tierID >= 3:
		return "high"
	case tierID == 2:
		return "medium"
	default: // tierID == 1
		return "low"
	}
}

// intigritiAssetValueFromBountyTierID maps an assetsCollection bountyTierId
// (as a gjson.Result) to a normalized asset value string. >= 3 -> high,
// 2 -> medium, 1 or 0 -> low. A missing tier yields an empty string.
func intigritiAssetValueFromBountyTierID(tier gjson.Result) string {
	if !tier.Exists() {
		return ""
	}
	id := tier.Int()
	switch {
	case id >= 3:
		return "high"
	case id == 2:
		return "medium"
	default: // id == 1 or 0
		return "low"
	}
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

// latestVersionEntry returns the entry with the highest `createdAt` value from
// the gjson array at arrayPath in body. Intigriti exposes several versioned
// arrays (bountyTables, rulesOfEngagements, inScopes, outOfScopes, faqs,
// assetsCollection) whose entries carry a `createdAt` timestamp. Comparison is
// done on the raw token string: ISO 8601 timestamps (the common case) sort
// chronologically under lexical comparison. Returns an invalid gjson.Result
// (Exists() == false) if the path is missing or empty.
func latestVersionEntry(body, arrayPath string) gjson.Result {
	arr := gjson.Get(body, arrayPath).Array()
	if len(arr) == 0 {
		return gjson.Result{}
	}
	var latest gjson.Result
	var latestTS string
	for _, entry := range arr {
		ts := entry.Get("createdAt").String()
		if ts > latestTS {
			latestTS = ts
			latest = entry
		}
	}
	return latest
}

// intPtr returns a pointer to v. Local helper for building metadata, mirroring
// the unexported scope.intPtr helper.
func intPtr(v int) *int { return &v }

// boolPtr returns a pointer to v. Local helper for building metadata, mirroring
// the unexported scope.boolPtr helper.
func boolPtr(v bool) *bool { return &v }

// extractIntigritiMetadata parses an Intigriti program detail JSON response body
// and returns the program-level metadata. It is defensive: missing or
// zero-valued numeric fields are left nil rather than producing zero pointers.
//
// The actual Intigriti researcher API (v1) returns a simpler shape than the
// sample provided during planning: rulesOfEngagement is a single object (not
// a versioned array), and bounty tables / invite / submissionCount / totalPayout
// are NOT available in the detail endpoint. Bounty min/max/currency come from
// the listing endpoint and are passed via the bountyInfo parameter.
func extractIntigritiMetadata(body string, isBBP bool, bi bountyInfo) *scope.ProgramMetadata {
	m := &scope.ProgramMetadata{}

	// 1. Classification & Context
	m.Title = gjson.Get(body, "name").String()
	m.Industry = bi.industry
	if m.Industry == "" {
		m.Industry = gjson.Get(body, "industry").String()
	}
	m.ProgramType = "bug-bounty"
	if !isBBP {
		m.ProgramType = "vdp"
	}
	m.IsBounty = boolPtr(isBBP)
	m.IsVDP = boolPtr(!isBBP)

	if v := gjson.Get(body, "confidentialityLevel.id"); v.Exists() {
		// 4 = public
		m.IsPublic = boolPtr(v.Int() == 4)
	}
	if v := gjson.Get(body, "status.id"); v.Exists() {
		// 4 = suspended
		m.IsDisabled = boolPtr(v.Int() == 4)
	}

	// 2. Rewards (from listing data, since detail endpoint doesn't include bounties)
	m.Currency = bi.currency
	if bi.minBounty > 0 {
		m.BountyRewardMin = intPtr(bi.minBounty)
	}
	if bi.maxBounty > 0 {
		m.BountyRewardMax = intPtr(bi.maxBounty)
	}

	// 3. Scope Rules
	// The actual API uses rulesOfEngagement (singular) with content.description
	// and content.testingRequirements.
	if roe := gjson.Get(body, "rulesOfEngagement"); roe.Exists() {
		if v := roe.Get("content.description").String(); v != "" {
			m.Rules = v
			m.RulesFormat = "markdown"
		}
		if tr := roe.Get("content.testingRequirements"); tr.Exists() {
			m.UserAgent = tr.Get("userAgent").String()
			m.RequestHeader = tr.Get("requestHeader").String()
			if v := tr.Get("automatedTooling"); v.Exists() {
				n := int(v.Int())
				m.AutomatedToolingLimit = &n
			}
		}
		if v := roe.Get("content.safeHarbour"); v.Exists() && v.Bool() {
			m.SafeHarbor = "true"
		}
		// intigritiMe grants @intigriti.me email account-creation capability
		if v := roe.Get("content.testingRequirements.intigritiMe"); v.Exists() && v.Bool() {
			m.CanCreateTestAccount = boolPtr(true)
		}
	}

	// 6. Program Stats — ScopesCount from domains.content (in-scope targets)
	if dc := gjson.Get(body, "domains.content"); dc.IsArray() {
		count := 0
		for _, a := range dc.Array() {
			if tier := a.Get("tier.id"); tier.Exists() && tier.Int() != 1 {
				// tier.id == 1 is "No Bounty" which may still be in-scope for VDPs
				count++
			} else if !a.Get("tier.id").Exists() {
				count++
			}
		}
		if count > 0 {
			m.ScopesCount = intPtr(count)
		}
	}

	return m
}
