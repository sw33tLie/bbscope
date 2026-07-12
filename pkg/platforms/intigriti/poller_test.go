package intigriti

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// TestExtractIntigritiMetadata verifies that extractIntigritiMetadata correctly
// parses the actual Intigriti researcher API v1 program detail response.
//
// The actual API response shape (confirmed via --debug-http) uses:
//   - rulesOfEngagement (singular object, not versioned array)
//   - confidentialityLevel.id / status.id (nested objects)
//   - domains.content for scope targets
//   - NO bountyTables, invite, submissionCount, totalPayout, credentials,
//     inScopes, outOfScopes, faqs, isVulnerabilityDisclosureProgram
//
// Bounty min/max/currency come from the listing endpoint via bountyInfo.
func TestExtractIntigritiMetadata(t *testing.T) {
	body := `{
		"id": "0acb1a74-c545-4941-ae57-7ca5fad9d449",
		"handle": "arm",
		"name": "Arm",
		"following": false,
		"confidentialityLevel": {"id": 4, "value": "Public"},
		"status": {"id": 3, "value": "Open"},
		"type": {"id": 1, "value": "Bug Bounty"},
		"domains": {
			"id": "5ba37c1d-1fea-4ef0-9a08-b95f191db5c9",
			"createdAt": 1741602671,
			"content": [
				{"id": "a1", "type": {"id": 6, "value": "Other"}, "endpoint": "Firmware: Mali CSF", "tier": {"id": 3, "value": "Tier 2"}, "description": "Arm GPU Firmware", "requiredSkills": []},
				{"id": "a2", "type": {"id": 6, "value": "Other"}, "endpoint": "Software: Mali GPU Kernel Driver", "tier": {"id": 2, "value": "Tier 3"}, "description": "Arm Mali GPU Kernel Driver", "requiredSkills": []}
			]
		},
		"rulesOfEngagement": {
			"attachments": [],
			"id": "16ff2df0-6d77-4397-96bf-4feccf3799d7",
			"createdAt": 1750681039,
			"content": {
				"description": "#### To be eligible for the Arm Bug Bounty Program, you **must not**:\n\n- Be a resident of a sanctioned country.\n\n#### By participating in this program, you agree **to**:\n\n* Respect the [Community Code of Conduct](https://go.intigriti.com/coc)\n* Respect the Intigriti [Terms and Conditions](https://go.intigriti.com/tac)",
				"testingRequirements": {
					"intigritiMe": false,
					"automatedTooling": 50,
					"userAgent": "",
					"requestHeader": "X-Intigriti-Username: {{USERNAME}}"
				},
				"safeHarbour": false
			}
		},
		"webLinks": {"detail": "https://app.intigriti.com/auth/dashboard?redirect=/programs/arm/arm/detail"},
		"industry": "Manufacturing - Consumer"
	}`

	bi := bountyInfo{
		minBounty: 500,
		maxBounty: 20000,
		currency:  "USD",
		industry:  "Manufacturing - Consumer",
	}

	m := extractIntigritiMetadata(body, true, bi)
	if m == nil {
		t.Fatal("extractIntigritiMetadata returned nil metadata")
	}

	// 1. Classification & Context
	if m.Title != "Arm" {
		t.Fatalf("Title = %q, want %q", m.Title, "Arm")
	}
	if m.Industry != "Manufacturing - Consumer" {
		t.Fatalf("Industry = %q, want %q", m.Industry, "Manufacturing - Consumer")
	}
	if m.ProgramType != "bug-bounty" {
		t.Fatalf("ProgramType = %q, want %q", m.ProgramType, "bug-bounty")
	}
	if m.IsBounty == nil || !*m.IsBounty {
		t.Fatalf("IsBounty = %s, want true", ptrBoolStr(m.IsBounty))
	}
	if m.IsVDP == nil || *m.IsVDP {
		t.Fatalf("IsVDP = %s, want false", ptrBoolStr(m.IsVDP))
	}
	if m.IsPublic == nil || !*m.IsPublic {
		t.Fatalf("IsPublic = %s, want true (confidentialityLevel.id=4)", ptrBoolStr(m.IsPublic))
	}
	if m.IsDisabled != nil && *m.IsDisabled {
		t.Fatalf("IsDisabled = %s, want false/nil (status.id=3, not 4)", ptrBoolStr(m.IsDisabled))
	}

	// 2. Rewards (from bountyInfo)
	if m.Currency != "USD" {
		t.Fatalf("Currency = %q, want %q", m.Currency, "USD")
	}
	if m.BountyRewardMin == nil || *m.BountyRewardMin != 500 {
		t.Fatalf("BountyRewardMin = %s, want 500", ptrIntStr(m.BountyRewardMin))
	}
	if m.BountyRewardMax == nil || *m.BountyRewardMax != 20000 {
		t.Fatalf("BountyRewardMax = %s, want 20000", ptrIntStr(m.BountyRewardMax))
	}

	// 3. Scope Rules
	if m.Rules == "" {
		t.Fatal("Rules is empty, want non-empty (rulesOfEngagement.content.description)")
	}
	if m.RulesFormat != "markdown" {
		t.Fatalf("RulesFormat = %q, want %q", m.RulesFormat, "markdown")
	}

	// 4. Testing Instructions
	if m.RequestHeader != "X-Intigriti-Username: {{USERNAME}}" {
		t.Fatalf("RequestHeader = %q, want %q", m.RequestHeader, "X-Intigriti-Username: {{USERNAME}}")
	}
	if m.AutomatedToolingLimit == nil || *m.AutomatedToolingLimit != 50 {
		t.Fatalf("AutomatedToolingLimit = %s, want 50", ptrIntStr(m.AutomatedToolingLimit))
	}
	// safeHarbour is false in this fixture
	if m.SafeHarbor != "" {
		t.Fatalf("SafeHarbor = %q, want empty (safeHarbour=false)", m.SafeHarbor)
	}

	// 5. Account Setup
	// intigritiMe is false -> CanCreateTestAccount should be nil
	if m.CanCreateTestAccount != nil && *m.CanCreateTestAccount {
		t.Fatalf("CanCreateTestAccount = %s, want nil/false (intigritiMe=false)", ptrBoolStr(m.CanCreateTestAccount))
	}

	// 6. Program Stats — ScopesCount from domains.content
	if m.ScopesCount == nil || *m.ScopesCount != 2 {
		t.Fatalf("ScopesCount = %s, want 2 (two assets in domains.content)", ptrIntStr(m.ScopesCount))
	}
}

// TestExtractIntigritiMetadata_SafeHarborAndIntigritiMe tests the safe harbor
// and intigritiMe=true paths.
func TestExtractIntigritiMetadata_SafeHarborAndIntigritiMe(t *testing.T) {
	body := `{
		"name": "Test Program",
		"confidentialityLevel": {"id": 1, "value": "InviteOnly"},
		"status": {"id": 3, "value": "Open"},
		"type": {"id": 1, "value": "Bug Bounty"},
		"domains": {"content": []},
		"rulesOfEngagement": {
			"content": {
				"description": "Rules text",
				"testingRequirements": {
					"intigritiMe": true,
					"automatedTooling": 1,
					"userAgent": "custom-ua",
					"requestHeader": "X-Test: value"
				},
				"safeHarbour": true
			}
		},
		"industry": "Software"
	}`

	bi := bountyInfo{minBounty: 50, maxBounty: 5000, currency: "EUR", industry: "Software"}

	m := extractIntigritiMetadata(body, true, bi)
	if m == nil {
		t.Fatal("extractIntigritiMetadata returned nil metadata")
	}

	if m.SafeHarbor != "true" {
		t.Fatalf("SafeHarbor = %q, want %q", m.SafeHarbor, "true")
	}
	if m.CanCreateTestAccount == nil || !*m.CanCreateTestAccount {
		t.Fatalf("CanCreateTestAccount = %s, want true (intigritiMe=true)", ptrBoolStr(m.CanCreateTestAccount))
	}
	if m.UserAgent != "custom-ua" {
		t.Fatalf("UserAgent = %q, want %q", m.UserAgent, "custom-ua")
	}
	if m.IsPublic == nil || *m.IsPublic {
		t.Fatalf("IsPublic = %s, want false (confidentialityLevel.id=1)", ptrBoolStr(m.IsPublic))
	}
}

func ptrIntStr(p *int) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *p)
}

func ptrBoolStr(p *bool) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *p)
}

// TestParseIntigritiMarkdownRewardGrids_Intel verifies that the Intel program's
// embedded markdown reward table (severity × asset category × currency amount)
// is parsed into one RewardGrid per asset category, with the Critical/High/
// Medium/Low Max slots populated from the "Up to $X" cells, and Min slots left
// nil (because "up to" only constrains the upper bound).
func TestParseIntigritiMarkdownRewardGrids_Intel(t *testing.T) {
	rules := `# Intel Bug Bounty Program

Some intro text.

| Vulnerability Severity | CVSS Score Range | Intel Cloud Services | Intel Software| Intel Firmware | Intel Hardware  |
| ------------------------------ | -------------------------- | ---------------------------- | -------------------- | ---------------------- | ---------------------- |
| Critical                           | 9.0 - 10.0                | Up to $5,000             | Up to $10,000 | Up to $30,000   | Up to $100,000 |
| High                               | 7.0 - 8.9                  | Up to $2,500             | Up to $5,000   | Up to $15,000   | Up to $30,000    |
| Medium                        | 4.0 - 6.9                  | Up to $750                 | Up to $1,500   | Up to $3,000    | Up to $5,000       |
| Low                                | 0.1 - 3.9                  | Up to $250                 | Up to $500      | Up to $1,000    | Up to $2,000      |

Some closing text.
`
	grids := parseIntigritiMarkdownRewardGrids(rules)
	wantDims := []string{"Intel Cloud Services", "Intel Software", "Intel Firmware", "Intel Hardware"}
	if len(grids) != len(wantDims) {
		t.Fatalf("len(grids) = %d, want %d; dims=%v", len(grids), len(wantDims), gridDimensionsLocal(grids))
	}
	for i, want := range wantDims {
		if grids[i].Dimension != want {
			t.Errorf("grids[%d].Dimension = %q, want %q", i, grids[i].Dimension, want)
		}
	}

	// Find the Intel Hardware grid (should have Critical Max = 100000).
	var hw *scope.RewardGrid
	for i := range grids {
		if grids[i].Dimension == "Intel Hardware" {
			hw = &grids[i]
			break
		}
	}
	if hw == nil {
		t.Fatalf("no Intel Hardware grid found")
	}
	if hw.BountyCriticalMax == nil || *hw.BountyCriticalMax != 100000 {
		t.Errorf("BountyCriticalMax = %s, want 100000", ptrIntStr(hw.BountyCriticalMax))
	}
	if hw.BountyCriticalMin != nil {
		t.Errorf("BountyCriticalMin = %s, want nil (\"Up to\" => only max)", ptrIntStr(hw.BountyCriticalMin))
	}
	if hw.BountyHighMax == nil || *hw.BountyHighMax != 30000 {
		t.Errorf("BountyHighMax = %s, want 30000", ptrIntStr(hw.BountyHighMax))
	}
	if hw.BountyMediumMax == nil || *hw.BountyMediumMax != 5000 {
		t.Errorf("BountyMediumMax = %s, want 5000", ptrIntStr(hw.BountyMediumMax))
	}
	if hw.BountyLowMax == nil || *hw.BountyLowMax != 2000 {
		t.Errorf("BountyLowMax = %s, want 2000", ptrIntStr(hw.BountyLowMax))
	}

	// The "CVSS Score Range" column should NOT produce a grid — its cells
	// (e.g. "9.0 - 10.0") lack a currency marker, so amountRegex skips them.
	for _, g := range grids {
		if strings.Contains(strings.ToLower(g.Dimension), "cvss") {
			t.Errorf("CVSS column leaked as grid: %q", g.Dimension)
		}
	}
}

// TestParseIntigritiMarkdownRewardGrids_ValidationTableSkipped verifies that
// "Severity × Time to validate" SLA tables are NOT parsed as reward grids,
// because their cells contain no currency amounts.
func TestParseIntigritiMarkdownRewardGrids_ValidationTableSkipped(t *testing.T) {
	rules := `**Validation times**

| Vulnerability Severity | Time to validate |
| -------- | -------- |
| Exceptional | 2 Working days |
| Critical | 2 Working days |
| High | 5 Working days |
`
	grids := parseIntigritiMarkdownRewardGrids(rules)
	if len(grids) != 0 {
		t.Fatalf("len(grids) = %d, want 0 (no currency amounts in table); got %v", len(grids), gridDimensionsLocal(grids))
	}
}

// TestParseIntigritiMarkdownRewardGrids_RangeSyntax verifies that "$X - $Y"
// ranges set both Min and Max.
func TestParseIntigritiMarkdownRewardGrids_RangeSyntax(t *testing.T) {
	rules := `| Severity | Web | API |
| --- | --- | --- |
| Critical | $5,000 - $10,000 | $1,000 |
| High | $2,500 - $5,000 | $500 |
`
	grids := parseIntigritiMarkdownRewardGrids(rules)
	if len(grids) != 2 {
		t.Fatalf("len(grids) = %d, want 2 (Web + API); got %v", len(grids), gridDimensionsLocal(grids))
	}
	var web *scope.RewardGrid
	for i := range grids {
		if grids[i].Dimension == "Web" {
			web = &grids[i]
		}
	}
	if web == nil {
		t.Fatalf("no Web grid found")
	}
	if web.BountyCriticalMin == nil || *web.BountyCriticalMin != 5000 {
		t.Errorf("BountyCriticalMin = %s, want 5000", ptrIntStr(web.BountyCriticalMin))
	}
	if web.BountyCriticalMax == nil || *web.BountyCriticalMax != 10000 {
		t.Errorf("BountyCriticalMax = %s, want 10000", ptrIntStr(web.BountyCriticalMax))
	}
	// Single value cell "$1,000" => min=max=1000.
	if web.BountyHighMin == nil || *web.BountyHighMin != 2500 {
		t.Errorf("BountyHighMin = %s, want 2500", ptrIntStr(web.BountyHighMin))
	}
	if web.BountyHighMax == nil || *web.BountyHighMax != 5000 {
		t.Errorf("BountyHighMax = %s, want 5000", ptrIntStr(web.BountyHighMax))
	}
	var api *scope.RewardGrid
	for i := range grids {
		if grids[i].Dimension == "API" {
			api = &grids[i]
		}
	}
	if api == nil {
		t.Fatalf("no API grid found")
	}
	if api.BountyCriticalMin == nil || *api.BountyCriticalMin != 1000 {
		t.Errorf("API BountyCriticalMin = %s, want 1000", ptrIntStr(api.BountyCriticalMin))
	}
	if api.BountyCriticalMax == nil || *api.BountyCriticalMax != 1000 {
		t.Errorf("API BountyCriticalMax = %s, want 1000 (single value => min=max)", ptrIntStr(api.BountyCriticalMax))
	}
}

// TestParseIntigritiMarkdownRewardGrids_NoTable verifies that rules text with
// no markdown table returns nil.
func TestParseIntigritiMarkdownRewardGrids_NoTable(t *testing.T) {
	rules := `Some prose rules with no table.

- bullet one
- bullet two

More text.`
	if g := parseIntigritiMarkdownRewardGrids(rules); len(g) != 0 {
		t.Fatalf("expected no grids, got %v", g)
	}
	if g := parseIntigritiMarkdownRewardGrids(""); len(g) != 0 {
		t.Fatalf("expected no grids for empty input, got %v", g)
	}
}

func gridDimensionsLocal(grids []scope.RewardGrid) []string {
	out := make([]string, len(grids))
	for i, g := range grids {
		out[i] = g.Dimension
	}
	return out
}
