package intigriti

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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

	// The Intigriti researcher API does not expose bountyTables / bounties[] in
	// the program detail response. For programs that publish a reward grid as
	// a markdown table inside the rulesOfEngagement.content.description, parse
	// it into structured RewardGrids so the WebUI / CLI render it like YWH / BC
	// grids. The listing-level min/max/currency remain authoritative for
	// BountyRewardMin/Max.
	if pData.Metadata != nil && len(pData.Metadata.RewardGrids) == 0 && pData.Metadata.Rules != "" {
		grids := parseIntigritiMarkdownRewardGrids(pData.Metadata.Rules)
		if len(grids) > 0 {
			pData.Metadata.RewardGrids = grids
		}
	}

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

// amountRegex matches a single monetary amount, optionally prefixed by a
// currency symbol ($, €, £) OR followed by a 3-letter currency code (USD,
// EUR, GBP). The capture group holds the bare number with possible thousands
// separators (e.g. "5,000"). Cells like "9.0 - 10.0" (CVSS score ranges)
// deliberately do NOT match because they lack a currency symbol/code, which
// lets us skip the "CVSS Score Range" column of Intigriti reward tables.
var amountRegex = regexp.MustCompile(`(?:\$|€|£)\s*([\d,]+(?:\.\d+)?)|([\d,]+(?:\.\d+)?)\s*(?:USD|EUR|GBP)\b`)

// tableRowRegex matches a single markdown table row like "| a | b | c |".
// Leading/trailing pipes are stripped by the caller via splitTableRow.
var tableRowRegex = regexp.MustCompile(`^\s*\|.*\|\s*$`)

// separatorRegex matches a markdown table separator row like "| --- | --- |".
var separatorRegex = regexp.MustCompile(`^\s*\|?\s*-{2,}\s*(\|\s*-{2,}\s*)*\|?\s*$`)

// parseIntigritiMarkdownRewardGrids scans the rules markdown for tables that
// look like bounty reward grids (severity rows × asset category columns with
// currency amounts) and parses them into scope.RewardGrid entries — one per
// asset category column.
//
// Tables that lack currency amounts (e.g. "Severity × Time to validate" SLA
// tables, "Identifier × Format" header tables) are skipped, because their
// cells do not contain a currency symbol/code that amountRegex recognises.
//
// Each produced grid has:
//   - Dimension = the column header (e.g. "Intel Hardware")
//   - BountyCriticalMin/Max, BountyHighMin/Max, BountyMediumMin/Max,
//     BountyLowMin/Max set from the matching severity row
//
// For cells like "Up to $5,000" only the Max is set (Min stays nil). For
// "$X - $Y" ranges both Min and Max are set.
func parseIntigritiMarkdownRewardGrids(rules string) []scope.RewardGrid {
	if rules == "" {
		return nil
	}
	lines := strings.Split(rules, "\n")

	var grids []scope.RewardGrid
	i := 0
	for i < len(lines) {
		// Find the start of a potential markdown table block.
		if !tableRowRegex.MatchString(lines[i]) {
			i++
			continue
		}
		// Collect consecutive table lines.
		blockStart := i
		for i < len(lines) && tableRowRegex.MatchString(lines[i]) {
			i++
		}
		block := lines[blockStart:i]
		// Parse the block; append any grids it produces.
		if g := parseRewardTableBlock(block); len(g) > 0 {
			grids = append(grids, g...)
		}
	}
	return grids
}

// parseRewardTableBlock parses a single markdown table block (header +
// separator + body rows) and returns RewardGrids for each column that
// carries currency amounts. Returns nil for non-bounty tables.
func parseRewardTableBlock(block []string) []scope.RewardGrid {
	if len(block) < 3 {
		return nil // need header + separator + at least one body row
	}
	header := splitTableRow(block[0])
	if !separatorRegex.MatchString(block[1]) {
		return nil
	}
	body := block[2:]

	// Per-column accumulator: tracks whether the column ever had a parsed
	// amount, and stores the partially-built RewardGrid keyed by header.
	type colState struct {
		header    string
		hasAmount bool
		grid      scope.RewardGrid
	}
	cols := make([]colState, len(header))
	for i, h := range header {
		cols[i] = colState{header: h, grid: scope.RewardGrid{Dimension: h}}
	}

	// Walk body rows. For each row, classify column 0 as a severity; for
	// every other column, parse the cell and (if it has an amount) record
	// it into the matching severity slot of that column's grid.
	for _, row := range body {
		cells := splitTableRow(row)
		if len(cells) < 2 {
			continue
		}
		sev := classifySeverity(cells[0])
		if sev == "" {
			continue
		}
		// cells[1..] are the value columns. cells may be shorter than cols;
		// we only fill columns that exist in both header and row.
		for ci := 1; ci < len(cols) && ci < len(cells); ci++ {
			min, max := parseAmountCell(cells[ci])
			if min == nil && max == nil {
				continue
			}
			cols[ci].hasAmount = true
			setSeverityBounty(&cols[ci].grid, sev, min, max)
		}
	}

	// Emit grids only for columns that contained at least one currency
	// amount. This naturally drops the "CVSS Score Range" column (no $/€/£
	// and no USD/EUR/GBP code) and the "Time to validate" column (no
	// currency amounts at all).
	var out []scope.RewardGrid
	for _, c := range cols {
		if c.hasAmount {
			out = append(out, c.grid)
		}
	}
	return out
}

// splitTableRow splits a markdown table row like "| a | b | c |" into
// ["a", "b", "c"], trimming whitespace from each cell.
func splitTableRow(row string) []string {
	s := strings.TrimSpace(row)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// classifySeverity maps the first cell of a reward-table row to a canonical
// severity token used by scope.RewardGrid: "critical", "high", "medium",
// "low", "info", or "exceptional". Returns "" if the cell is not a severity
// label (e.g. CVSS range, validation time).
func classifySeverity(cell string) string {
	// Take the first whitespace-delimited word, lowercased. This handles
	// cells like "Critical", "Critical (9.0 - 10.0)", "Low (0.1 - 3.9)".
	fields := strings.Fields(cell)
	if len(fields) == 0 {
		return ""
	}
	w := strings.ToLower(fields[0])
	// Strip any trailing punctuation like "critical:" → "critical".
	w = strings.TrimRight(w, ":.,;")
	switch w {
	case "exceptional":
		return "exceptional"
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "info", "informational":
		return "info"
	}
	// Some Intigriti programs use "P1"/"P2"/... instead of severity names.
	switch w {
	case "p1":
		return "critical"
	case "p2":
		return "high"
	case "p3":
		return "medium"
	case "p4":
		return "low"
	case "p5":
		return "info"
	}
	return ""
}

// setSeverityBounty sets the min/max slot of g for the given severity.
// sev is one of "info", "low", "medium", "high", "critical", "exceptional".
// A nil min leaves Min unset; a nil max leaves Max unset. A non-nil value
// only overrides the existing slot if it is currently unset (first-write-wins
// across multiple body rows for the same severity — Intigriti tables have
// one row per severity, so this is purely defensive).
func setSeverityBounty(g *scope.RewardGrid, sev string, min, max *int) {
	switch sev {
	case "exceptional":
		if g.BountyExceptionalMin == nil {
			g.BountyExceptionalMin = min
		}
		if g.BountyExceptionalMax == nil {
			g.BountyExceptionalMax = max
		}
	case "critical":
		if g.BountyCriticalMin == nil {
			g.BountyCriticalMin = min
		}
		if g.BountyCriticalMax == nil {
			g.BountyCriticalMax = max
		}
	case "high":
		if g.BountyHighMin == nil {
			g.BountyHighMin = min
		}
		if g.BountyHighMax == nil {
			g.BountyHighMax = max
		}
	case "medium":
		if g.BountyMediumMin == nil {
			g.BountyMediumMin = min
		}
		if g.BountyMediumMax == nil {
			g.BountyMediumMax = max
		}
	case "low":
		if g.BountyLowMin == nil {
			g.BountyLowMin = min
		}
		if g.BountyLowMax == nil {
			g.BountyLowMax = max
		}
	case "info":
		if g.BountyInfoMin == nil {
			g.BountyInfoMin = min
		}
		if g.BountyInfoMax == nil {
			g.BountyInfoMax = max
		}
	}
}

// parseAmountCell parses a single reward-table cell into (min, max) integer
// pointers. Returns (nil, nil) if no currency amount is found in the cell.
//
// Behaviour:
//   - "Up to $5,000" → (nil, 5000)
//   - "$5,000 - $10,000" → (5000, 10000)
//   - "$5,000" (no prefix) → (5000, 5000)
//   - "From $5,000" → (5000, nil)
//   - "9.0 - 10.0" → (nil, nil)  (no currency marker)
//   - "2 Working days" → (nil, nil)
func parseAmountCell(cell string) (min, max *int) {
	matches := amountRegex.FindAllStringSubmatch(cell, -1)
	if len(matches) == 0 {
		return nil, nil
	}
	values := make([]int, 0, 2)
	for _, m := range matches {
		// m[1] is the capture for the "$X" form; m[2] is for the "X USD" form.
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		raw = strings.ReplaceAll(raw, ",", "")
		// Drop any decimal part — bounties are whole units, and a fractional
		// value here would be a CVSS-like number that slipped through.
		if i := strings.Index(raw, "."); i >= 0 {
			raw = raw[:i]
		}
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			continue
		}
		values = append(values, n)
	}
	if len(values) == 0 {
		return nil, nil
	}

	lower := strings.ToLower(strings.TrimSpace(cell))
	switch {
	case strings.HasPrefix(lower, "up to"), strings.HasPrefix(lower, "max "):
		// Only an upper bound.
		return nil, &values[0]
	case strings.HasPrefix(lower, "from"), strings.HasPrefix(lower, "starting at"), strings.HasPrefix(lower, "min "), strings.HasPrefix(lower, "minimum"):
		// Only a lower bound.
		return &values[0], nil
	case len(values) >= 2:
		return &values[0], &values[1]
	default:
		// Single value, no prefix — treat as both min and max (matches the
		// YWH convention where a single bounty value is min=max).
		v := values[0]
		return &v, &v
	}
}
