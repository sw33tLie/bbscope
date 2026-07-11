package scope

import "strconv"

// RewardGrid captures bounties for a given dimension across severity levels.
// The dimension varies by platform:
//   - YesWeHack: asset value ("default", "low", "medium", "high", "critical")
//   - Bugcrowd:  scope group name ("Core Assets", "Primary Assets", ...)
//   - Intigriti: asset category (e.g. "Intel Cloud Services", "Intel Hardware")
//     parsed from the markdown reward table embedded in the program rules —
//     the researcher API does not expose structured bounty tables (only the
//     company API does, which bbscope cannot reach with a researcher token).
//
// Severity levels are normalized across platforms:
//   - YesWeHack: low/medium/high/critical (4 levels, single values → min=max)
//   - Bugcrowd:  P1=critical, P2=high, P3=medium, P4=low, P5=info (5 levels, min+max ranges)
//   - Intigriti:  CVSS ranges → low/medium/high/critical/exceptional (5 levels, min+max)
type RewardGrid struct {
	Dimension            string `json:"dimension,omitempty"`
	BountyInfoMin        *int   `json:"bounty_info_min,omitempty"` // BC P5 / Informational
	BountyInfoMax        *int   `json:"bounty_info_max,omitempty"`
	BountyLowMin         *int   `json:"bounty_low_min,omitempty"`
	BountyLowMax         *int   `json:"bounty_low_max,omitempty"`
	BountyMediumMin      *int   `json:"bounty_medium_min,omitempty"`
	BountyMediumMax      *int   `json:"bounty_medium_max,omitempty"`
	BountyHighMin        *int   `json:"bounty_high_min,omitempty"`
	BountyHighMax        *int   `json:"bounty_high_max,omitempty"`
	BountyCriticalMin    *int   `json:"bounty_critical_min,omitempty"`
	BountyCriticalMax    *int   `json:"bounty_critical_max,omitempty"`
	BountyExceptionalMin *int   `json:"bounty_exceptional_min,omitempty"` // Intigriti only (CVSS 9.5-10.0)
	BountyExceptionalMax *int   `json:"bounty_exceptional_max,omitempty"`
}

// ProgramMetadata captures program-level info that bbscope previously fetched
// from each platform's API and then discarded. Pointer fields are nil when the
// platform does not expose them, so callers can distinguish "unset" from "zero".
//
// A nil ProgramData.Metadata means the poller did not produce any metadata
// for this program.
//
// Fields are organized by the AI agent workflow:
//  1. Classification & Context — what am I hunting and for whom?
//  2. Rewards — what's the payoff and how do I prioritize?
//  3. Scope Rules — what's fair game and what gets rejected?
//  4. Testing Instructions — how do I test ethically?
//  5. Account Setup — how do I get access?
//  6. Program Stats — is this program worth hunting?
type ProgramMetadata struct {
	// 1. Classification & Context
	Title       string `json:"title,omitempty"`
	Tagline     string `json:"tagline,omitempty"`
	CompanyName string `json:"company_name,omitempty"`
	Industry    string `json:"industry,omitempty"`
	ProgramType string `json:"program_type,omitempty"` // "bug-bounty", "vdp", ...
	IsPublic    *bool  `json:"is_public,omitempty"`
	IsBounty    *bool  `json:"is_bounty,omitempty"`
	IsVDP       *bool  `json:"is_vdp,omitempty"`
	IsDisabled  *bool  `json:"is_disabled,omitempty"`

	// 2. Rewards
	Currency        string       `json:"currency,omitempty"`
	BountyRewardMin *int         `json:"bounty_reward_min,omitempty"`
	BountyRewardMax *int         `json:"bounty_reward_max,omitempty"`
	RewardGrids     []RewardGrid `json:"reward_grids,omitempty"`

	// 3. Scope Rules
	Rules                        string   `json:"rules,omitempty"`
	RulesFormat                  string   `json:"rules_format,omitempty"` // "markdown" | "html"
	InScopeDescription           string   `json:"in_scope_description,omitempty"`
	QualifyingVulnerabilities    []string `json:"qualifying_vulnerabilities,omitempty"`
	NonQualifyingVulnerabilities []string `json:"non_qualifying_vulnerabilities,omitempty"`
	OutOfScopeSummary            []string `json:"out_of_scope_summary,omitempty"`
	FAQs                         string   `json:"faqs,omitempty"`

	// 4. Testing Instructions
	UserAgent             string   `json:"user_agent,omitempty"`
	RequestHeader         string   `json:"request_header,omitempty"`
	AutomatedToolingLimit *int     `json:"automated_tooling_limit,omitempty"`
	VPNRequired           *bool    `json:"vpn_required,omitempty"`
	VNPIPs                []string `json:"vpn_ips,omitempty"`
	Secured               *bool    `json:"secured,omitempty"`     // 2FA required
	SafeHarbor            string   `json:"safe_harbor,omitempty"` // "full"/"partial"/"none"/"true"/""

	// 5. Account Setup
	AccountAccess        string `json:"account_access,omitempty"`
	CanCreateTestAccount *bool  `json:"can_create_test_account,omitempty"`

	// 6. Program Stats
	ReportsCount         *int     `json:"reports_count,omitempty"`
	TotalPayout          *int     `json:"total_payout,omitempty"`
	TotalPayoutCurrency  string   `json:"total_payout_currency,omitempty"`
	AvgReward            *int     `json:"avg_reward,omitempty"`
	AvgFirstResponseDays *float64 `json:"avg_first_response_days,omitempty"`
	ScopesCount          *int     `json:"scopes_count,omitempty"`
	Tags                 []string `json:"tags,omitempty"`
}

// HasRewardInfo reports whether this metadata carries any reward data worth
// showing. Useful for skipping empty sections in the CLI/web views.
func (m *ProgramMetadata) HasRewardInfo() bool {
	if m == nil {
		return false
	}
	return m.Currency != "" || m.BountyRewardMin != nil || m.BountyRewardMax != nil ||
		len(m.RewardGrids) > 0
}

// FormatBountySlot renders a single severity slot of a RewardGrid as a
// short string suitable for table cells. It considers both Min and Max so
// that platforms which only set one bound (e.g. Intigriti's "Up to $X" cells,
// which set only Max) still display a meaningful value instead of "-".
//
//   - both nil              -> "-"
//   - min == nil, max set   -> strconv(max)             ("Up to $X" case)
//   - min set, max == nil   -> strconv(min)             ("From $X" case)
//   - min == max            -> strconv(min)             (YWH single-value case)
//   - min != max (both set) -> "min - max"              (BC / Intigriti range case)
func FormatBountySlot(min, max *int) string {
	switch {
	case min == nil && max == nil:
		return "-"
	case min == nil:
		return strconv.Itoa(*max)
	case max == nil:
		return strconv.Itoa(*min)
	case *min == *max:
		return strconv.Itoa(*min)
	default:
		return strconv.Itoa(*min) + " - " + strconv.Itoa(*max)
	}
}

// intPtr returns a pointer to the given int. Helper for building metadata.
func intPtr(v int) *int { return &v }

// boolPtr returns a pointer to the given bool. Helper for building metadata.
func boolPtr(v bool) *bool { return &v }
