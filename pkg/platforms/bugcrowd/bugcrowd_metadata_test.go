package bugcrowd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// TestExtractBugcrowdMetadata verifies that extractBugcrowdMetadata correctly
// parses a condensed Bugcrowd engagement JSON (based on the Nubank Brasil
// sample) into a scope.ProgramMetadata.
func TestExtractBugcrowdMetadata(t *testing.T) {
	body := `{
		"data": {
			"brief": {
				"name": "Nubank Brasil Managed Bug Bounty Program",
				"tagline": "Nubank Brasil is in a mission...",
				"description": "<h2>Access</h2><p>Create an account...</p><h2>Program Rules</h2>..."
			},
			"engagement": {
				"state": "in_progress"
			},
			"scope": [
				{
					"name": "Core Assets",
					"inScope": true,
					"targets": [
						{"name": "api.nubank.com", "tags": [{"name": "Android"}]}
					],
					"rewardRangeData": {
						"1": {"min": 2000, "max": 4000},
						"2": {"min": 1000, "max": 2000},
						"3": {"min": 300, "max": 600},
						"4": {"min": 50, "max": 100}
					}
				},
				{
					"name": "Out of Scope",
					"inScope": false,
					"targets": [
						{"name": "*.nuinternational.com", "tags": []}
					]
				}
			]
		},
		"industryName": "Finance",
		"engagementTypeDetail": {
			"productLabel": "Bug Bounty",
			"iconVariant": "bug-bounty"
		},
		"engagementHiddenData": {
			"isPrivate": false
		},
		"safeHarborStatus": {
			"status": "full"
		},
		"credentialsUrl": null
	}`

	m := extractBugcrowdMetadata(body, true)
	if m == nil {
		t.Fatal("extractBugcrowdMetadata returned nil metadata")
	}

	// 1. Classification & Context
	if m.Title != "Nubank Brasil Managed Bug Bounty Program" {
		t.Fatalf("Title = %q, want %q", m.Title, "Nubank Brasil Managed Bug Bounty Program")
	}
	if m.Industry != "Finance" {
		t.Fatalf("Industry = %q, want %q", m.Industry, "Finance")
	}
	if m.ProgramType != "bug-bounty" {
		t.Fatalf("ProgramType = %q, want %q", m.ProgramType, "bug-bounty")
	}
	if m.IsPublic == nil || !*m.IsPublic {
		t.Fatalf("IsPublic = %s, want true (isPrivate=false -> IsPublic=true)", ptrBoolStr(m.IsPublic))
	}
	if m.IsBounty == nil || !*m.IsBounty {
		t.Fatalf("IsBounty = %s, want true (iconVariant=bug-bounty)", ptrBoolStr(m.IsBounty))
	}
	if m.IsDisabled == nil || *m.IsDisabled {
		t.Fatalf("IsDisabled = %s, want false (state=in_progress)", ptrBoolStr(m.IsDisabled))
	}
	if m.SafeHarbor != "full" {
		t.Fatalf("SafeHarbor = %q, want %q", m.SafeHarbor, "full")
	}
	// credentialsUrl is null -> CanCreateTestAccount stays nil (treated as false).
	if m.CanCreateTestAccount != nil && *m.CanCreateTestAccount {
		t.Fatalf("CanCreateTestAccount = %s, want false/nil (credentialsUrl is null)", ptrBoolStr(m.CanCreateTestAccount))
	}

	// 2. Rewards
	if len(m.RewardGrids) < 1 {
		t.Fatalf("len(RewardGrids) = %d, want >= 1", len(m.RewardGrids))
	}
	var coreGrid *scope.RewardGrid
	for i := range m.RewardGrids {
		if strings.Contains(m.RewardGrids[i].Dimension, "Core Assets") {
			coreGrid = &m.RewardGrids[i]
			break
		}
	}
	if coreGrid == nil {
		t.Fatalf("no RewardGrid with Dimension containing 'Core Assets'; got %v", gridDimensions(m.RewardGrids))
	}
	if coreGrid.BountyCriticalMin == nil || *coreGrid.BountyCriticalMin != 2000 {
		t.Fatalf("Core Assets BountyCriticalMin = %s, want 2000 (P1.min)", ptrIntStr(coreGrid.BountyCriticalMin))
	}
	if coreGrid.BountyCriticalMax == nil || *coreGrid.BountyCriticalMax != 4000 {
		t.Fatalf("Core Assets BountyCriticalMax = %s, want 4000 (P1.max)", ptrIntStr(coreGrid.BountyCriticalMax))
	}
	if m.BountyRewardMin == nil || *m.BountyRewardMin != 50 {
		t.Fatalf("BountyRewardMin = %s, want 50 (min across all groups)", ptrIntStr(m.BountyRewardMin))
	}
	if m.BountyRewardMax == nil || *m.BountyRewardMax != 4000 {
		t.Fatalf("BountyRewardMax = %s, want 4000 (max across all groups)", ptrIntStr(m.BountyRewardMax))
	}

	// 3. Scope Rules
	if m.RulesFormat != "html" {
		t.Fatalf("RulesFormat = %q, want %q", m.RulesFormat, "html")
	}
	if len(m.OutOfScopeSummary) == 0 {
		t.Fatal("OutOfScopeSummary is empty, want to contain '*.nuinternational.com'")
	}
	oosFound := false
	for _, s := range m.OutOfScopeSummary {
		if strings.Contains(s, "*.nuinternational.com") {
			oosFound = true
			break
		}
	}
	if !oosFound {
		t.Fatalf("OutOfScopeSummary = %v, want to contain '*.nuinternational.com'", m.OutOfScopeSummary)
	}

	// 6. Program Stats - Tags
	if len(m.Tags) == 0 {
		t.Fatal("Tags is empty, want to contain 'Android'")
	}
	tagFound := false
	for _, tag := range m.Tags {
		if tag == "Android" {
			tagFound = true
			break
		}
	}
	if !tagFound {
		t.Fatalf("Tags = %v, want to contain 'Android'", m.Tags)
	}
}

// ptrIntStr renders an *int for failure messages, tolerating nil.
func ptrIntStr(p *int) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *p)
}

// ptrBoolStr renders a *bool for failure messages, tolerating nil.
func ptrBoolStr(p *bool) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *p)
}

// gridDimensions returns the list of Dimension values for debug output.
func gridDimensions(grids []scope.RewardGrid) []string {
	out := make([]string, len(grids))
	for i, g := range grids {
		out[i] = g.Dimension
	}
	return out
}
