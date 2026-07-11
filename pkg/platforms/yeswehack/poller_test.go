package yeswehack

import "testing"

// TestExtractYWHMetadata verifies that extractYWHMetadata correctly parses a
// condensed YesWeHack program API response (modelled on the Ferrero bug bounty
// program) into a scope.ProgramMetadata struct. The long `rules`/`rules_html`
// payloads from the real response are replaced with a short placeholder; the
// reward_grid_very_low object is included with all-null severities to exercise
// the skip-zero-grid logic.
func TestExtractYWHMetadata(t *testing.T) {
	const body = `{
  "title": "Ferrero International S.A. - Bug Bounty Program",
  "type": "bug-bounty",
  "public": false,
  "bounty": true,
  "vdp": false,
  "disabled": false,
  "secured": true,
  "bounty_reward_min": 10,
  "bounty_reward_max": 2500,
  "scopes_count": 48,
  "business_unit": {
    "currency": "EUR"
  },
  "reward_grid_very_low": {
    "bounty_low": null,
    "bounty_medium": null,
    "bounty_high": null,
    "bounty_critical": null
  },
  "reward_grid_default": {
    "bounty_low": 50,
    "bounty_medium": 250,
    "bounty_high": 800,
    "bounty_critical": 2000
  },
  "reward_grid_critical": {
    "bounty_low": 50,
    "bounty_medium": 100,
    "bounty_high": 1000,
    "bounty_critical": 2500
  },
  "reports_count": 313,
  "stats": {
    "total_reports": 313,
    "average_first_time_response": 1.0,
    "average_reward": 450
  },
  "rules": "# Ferrero Bug Bounty Program Rules\n\nPlease read the rules carefully before reporting.",
  "qualifying_vulnerability": [
    "Remote code execution (RCE)",
    "SQL Injection",
    "Cross-site scripting (XSS)"
  ],
  "non_qualifying_vulnerability": [
    "Tabnabbing",
    "Clickjacking"
  ],
  "out_of_scope": [
    "Third-party applications",
    "Physical security"
  ],
  "account_access": "Test accounts are available at https://yeswehack.ninja/ferrero",
  "user_agent": "-BugBounty-ferrero-international-s.a.-31337",
  "vpn_active": false,
  "tags": ["food", "manufacturing"]
}`

	m := extractYWHMetadata(body, true)
	if m == nil {
		t.Fatal("extractYWHMetadata returned nil metadata")
	}

	// 1. Classification & Context
	if m.Title != "Ferrero International S.A. - Bug Bounty Program" {
		t.Errorf("Title = %q, want %q", m.Title, "Ferrero International S.A. - Bug Bounty Program")
	}
	if m.ProgramType != "bug-bounty" {
		t.Errorf("ProgramType = %q, want %q", m.ProgramType, "bug-bounty")
	}
	if m.IsBounty == nil {
		t.Fatal("IsBounty is nil, want pointer to true")
	} else if *m.IsBounty != true {
		t.Errorf("IsBounty = %v, want true", *m.IsBounty)
	}
	if m.IsVDP == nil {
		t.Fatal("IsVDP is nil, want pointer to false")
	} else if *m.IsVDP != false {
		t.Errorf("IsVDP = %v, want false", *m.IsVDP)
	}
	if m.IsPublic == nil {
		t.Fatal("IsPublic is nil, want pointer to false")
	} else if *m.IsPublic != false {
		t.Errorf("IsPublic = %v, want false", *m.IsPublic)
	}
	if m.Secured == nil {
		t.Fatal("Secured is nil, want pointer to true")
	} else if *m.Secured != true {
		t.Errorf("Secured = %v, want true", *m.Secured)
	}

	// 2. Rewards
	if m.Currency != "EUR" {
		t.Errorf("Currency = %q, want %q", m.Currency, "EUR")
	}
	if m.BountyRewardMin == nil {
		t.Fatal("BountyRewardMin is nil, want pointer to 10")
	} else if *m.BountyRewardMin != 10 {
		t.Errorf("BountyRewardMin = %d, want 10", *m.BountyRewardMin)
	}
	if m.BountyRewardMax == nil {
		t.Fatal("BountyRewardMax is nil, want pointer to 2500")
	} else if *m.BountyRewardMax != 2500 {
		t.Errorf("BountyRewardMax = %d, want 2500", *m.BountyRewardMax)
	}
	if m.ScopesCount == nil {
		t.Fatal("ScopesCount is nil, want pointer to 48")
	} else if *m.ScopesCount != 48 {
		t.Errorf("ScopesCount = %d, want 48", *m.ScopesCount)
	}

	// Reward grids: at least "default" and "critical"; the all-null
	// reward_grid_very_low must be skipped.
	if len(m.RewardGrids) < 2 {
		t.Fatalf("len(RewardGrids) = %d, want >= 2", len(m.RewardGrids))
	}
	var foundCritical bool
	for _, rg := range m.RewardGrids {
		if rg.Dimension == "critical" {
			foundCritical = true
			if rg.BountyCriticalMin == nil {
				t.Fatal("critical grid BountyCriticalMin is nil, want pointer to 2500")
			} else if *rg.BountyCriticalMin != 2500 {
				t.Errorf("critical grid BountyCriticalMin = %d, want 2500", *rg.BountyCriticalMin)
			}
		}
		if rg.Dimension == "very_low" {
			t.Error("very_low reward grid should have been skipped (all severities null)")
		}
	}
	if !foundCritical {
		t.Error("no reward grid with dimension 'critical' found")
	}

	// 6. Program Stats
	if m.ReportsCount == nil {
		t.Fatal("ReportsCount is nil, want pointer to 313")
	} else if *m.ReportsCount != 313 {
		t.Errorf("ReportsCount = %d, want 313", *m.ReportsCount)
	}
	if m.AvgFirstResponseDays == nil {
		t.Fatal("AvgFirstResponseDays is nil, want pointer to 1.0")
	} else if *m.AvgFirstResponseDays != 1.0 {
		t.Errorf("AvgFirstResponseDays = %v, want 1.0", *m.AvgFirstResponseDays)
	}

	// 3. Scope Rules
	if len(m.QualifyingVulnerabilities) == 0 {
		t.Error("QualifyingVulnerabilities is empty, want non-empty")
	} else {
		found := false
		for _, q := range m.QualifyingVulnerabilities {
			if q == "Remote code execution (RCE)" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("QualifyingVulnerabilities does not contain 'Remote code execution (RCE)', got %v", m.QualifyingVulnerabilities)
		}
	}
	if len(m.NonQualifyingVulnerabilities) == 0 {
		t.Error("NonQualifyingVulnerabilities is empty, want non-empty")
	} else {
		found := false
		for _, n := range m.NonQualifyingVulnerabilities {
			if n == "Tabnabbing" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("NonQualifyingVulnerabilities does not contain 'Tabnabbing', got %v", m.NonQualifyingVulnerabilities)
		}
	}
	if len(m.OutOfScopeSummary) == 0 {
		t.Error("OutOfScopeSummary is empty, want non-empty")
	}
	if m.Rules == "" {
		t.Error("Rules is empty, want non-empty")
	}
	if m.RulesFormat != "markdown" {
		t.Errorf("RulesFormat = %q, want %q", m.RulesFormat, "markdown")
	}

	// 4. Testing Instructions
	if m.UserAgent != "-BugBounty-ferrero-international-s.a.-31337" {
		t.Errorf("UserAgent = %q, want %q", m.UserAgent, "-BugBounty-ferrero-international-s.a.-31337")
	}

	// 5. Account Setup
	if m.CanCreateTestAccount == nil {
		t.Fatal("CanCreateTestAccount is nil, want pointer to true")
	} else if *m.CanCreateTestAccount != true {
		t.Errorf("CanCreateTestAccount = %v, want true", *m.CanCreateTestAccount)
	}
}
