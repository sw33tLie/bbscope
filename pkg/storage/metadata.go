package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// UpsertProgramMetadata persists program-level metadata for the program with
// the given URL. The program must already exist (created by UpsertProgramEntries
// or getOrCreateProgram). A nil metadata is a no-op.
func (d *DB) UpsertProgramMetadata(ctx context.Context, programURL string, md *scope.ProgramMetadata) error {
	if md == nil {
		return nil
	}

	var programID int64
	err := d.sql.QueryRowContext(ctx, `SELECT id FROM programs WHERE url = $1`, programURL).Scan(&programID)
	if err != nil {
		return fmt.Errorf("locating program for metadata upsert (%s): %w", programURL, err)
	}

	rewardGridsJSON, _ := json.Marshal(md.RewardGrids)
	qualVulnsJSON, _ := json.Marshal(md.QualifyingVulnerabilities)
	nonQualVulnsJSON, _ := json.Marshal(md.NonQualifyingVulnerabilities)
	oosJSON, _ := json.Marshal(md.OutOfScopeSummary)
	vpnIPsJSON, _ := json.Marshal(md.VNPIPs)
	tagsJSON, _ := json.Marshal(md.Tags)

	_, err = d.sql.ExecContext(ctx, `
		INSERT INTO program_metadata (
			program_id, title, tagline, company_name, industry, program_type,
			is_public, is_bounty, is_vdp, is_disabled,
			currency, bounty_reward_min, bounty_reward_max, reward_grids,
			rules, rules_format, in_scope_description,
			qualifying_vulnerabilities, non_qualifying_vulnerabilities, out_of_scope_summary, faqs,
			user_agent, request_header, automated_tooling_limit, vpn_required, vpn_ips, secured, safe_harbor,
			account_access, can_create_test_account,
			reports_count, total_payout, total_payout_currency, avg_reward, avg_first_response_days, scopes_count, tags,
			first_seen_at, last_seen_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)
		ON CONFLICT (program_id) DO UPDATE SET
			title                           = excluded.title,
			tagline                         = excluded.tagline,
			company_name                    = excluded.company_name,
			industry                        = excluded.industry,
			program_type                    = excluded.program_type,
			is_public                       = excluded.is_public,
			is_bounty                       = excluded.is_bounty,
			is_vdp                          = excluded.is_vdp,
			is_disabled                     = excluded.is_disabled,
			currency                        = excluded.currency,
			bounty_reward_min               = excluded.bounty_reward_min,
			bounty_reward_max               = excluded.bounty_reward_max,
			reward_grids                    = excluded.reward_grids,
			rules                           = excluded.rules,
			rules_format                    = excluded.rules_format,
			in_scope_description            = excluded.in_scope_description,
			qualifying_vulnerabilities      = excluded.qualifying_vulnerabilities,
			non_qualifying_vulnerabilities  = excluded.non_qualifying_vulnerabilities,
			out_of_scope_summary            = excluded.out_of_scope_summary,
			faqs                            = excluded.faqs,
			user_agent                      = excluded.user_agent,
			request_header                  = excluded.request_header,
			automated_tooling_limit         = excluded.automated_tooling_limit,
			vpn_required                    = excluded.vpn_required,
			vpn_ips                         = excluded.vpn_ips,
			secured                         = excluded.secured,
			safe_harbor                     = excluded.safe_harbor,
			account_access                  = excluded.account_access,
			can_create_test_account         = excluded.can_create_test_account,
			reports_count                   = excluded.reports_count,
			total_payout                    = excluded.total_payout,
			total_payout_currency            = excluded.total_payout_currency,
			avg_reward                      = excluded.avg_reward,
			avg_first_response_days         = excluded.avg_first_response_days,
			scopes_count                    = excluded.scopes_count,
			tags                            = excluded.tags,
			last_seen_at                    = CURRENT_TIMESTAMP
	`,
		programID,
		md.Title,
		md.Tagline,
		md.CompanyName,
		md.Industry,
		md.ProgramType,
		boolToIntPtr(md.IsPublic),
		boolToIntPtr(md.IsBounty),
		boolToIntPtr(md.IsVDP),
		boolToIntPtr(md.IsDisabled),
		md.Currency,
		md.BountyRewardMin,
		md.BountyRewardMax,
		rewardGridsJSON,
		md.Rules,
		md.RulesFormat,
		md.InScopeDescription,
		qualVulnsJSON,
		nonQualVulnsJSON,
		oosJSON,
		md.FAQs,
		md.UserAgent,
		md.RequestHeader,
		md.AutomatedToolingLimit,
		boolToIntPtr(md.VPNRequired),
		vpnIPsJSON,
		boolToIntPtr(md.Secured),
		md.SafeHarbor,
		md.AccountAccess,
		boolToIntPtr(md.CanCreateTestAccount),
		md.ReportsCount,
		md.TotalPayout,
		md.TotalPayoutCurrency,
		md.AvgReward,
		md.AvgFirstResponseDays,
		md.ScopesCount,
		tagsJSON,
	)
	return err
}

// GetProgramMetadata fetches metadata for a program by its DB id. Returns
// (nil, nil) when no metadata row exists.
func (d *DB) GetProgramMetadata(ctx context.Context, programID int64) (*scope.ProgramMetadata, error) {
	row := d.sql.QueryRowContext(ctx, metadataSelectSQL+` WHERE program_id = $1`, programID)
	return scanMetadata(row)
}

// GetProgramMetadataByPlatformHandle fetches metadata via platform+handle.
func (d *DB) GetProgramMetadataByPlatformHandle(ctx context.Context, platform, handle string) (*scope.ProgramMetadata, error) {
	row := d.sql.QueryRowContext(ctx,
		metadataSelectSQL+`
		WHERE program_id = (
			SELECT id FROM programs
			WHERE LOWER(platform) = LOWER($1) AND LOWER(handle) = LOWER($2)
			LIMIT 1
		)`, platform, handle)
	return scanMetadata(row)
}

const metadataSelectSQL = `
	SELECT
		COALESCE(title,''), COALESCE(tagline,''), COALESCE(company_name,''), COALESCE(industry,''),
		COALESCE(program_type,''),
		is_public, is_bounty, is_vdp, is_disabled,
		COALESCE(currency,''), bounty_reward_min, bounty_reward_max, reward_grids,
		COALESCE(rules,''), COALESCE(rules_format,''), COALESCE(in_scope_description,''),
		qualifying_vulnerabilities, non_qualifying_vulnerabilities, out_of_scope_summary, COALESCE(faqs,''),
		COALESCE(user_agent,''), COALESCE(request_header,''), automated_tooling_limit,
		vpn_required, vpn_ips, secured, COALESCE(safe_harbor,''),
		COALESCE(account_access,''), can_create_test_account,
		reports_count, total_payout, COALESCE(total_payout_currency,''), avg_reward,
		avg_first_response_days, scopes_count, tags
	FROM program_metadata
`

type metadataScanner interface {
	Scan(dest ...interface{}) error
}

func scanMetadata(row metadataScanner) (*scope.ProgramMetadata, error) {
	var md scope.ProgramMetadata
	var (
		isPublic, isBounty, isVDP, isDisabled,
		canCreateTest, vpnRequired, secured sql.NullInt64
		bountyMin, bountyMax, reportsCount,
		totalPayout, avgReward, automatedToolingLimit, scopesCount sql.NullInt64
		avgFirstResponse                                               sql.NullFloat64
		rewardGrids, qualVulns, nonQualVulns, oosSummary, vpnIPs, tags []byte
	)
	if err := row.Scan(
		&md.Title, &md.Tagline, &md.CompanyName, &md.Industry, &md.ProgramType,
		&isPublic, &isBounty, &isVDP, &isDisabled,
		&md.Currency, &bountyMin, &bountyMax, &rewardGrids,
		&md.Rules, &md.RulesFormat, &md.InScopeDescription,
		&qualVulns, &nonQualVulns, &oosSummary, &md.FAQs,
		&md.UserAgent, &md.RequestHeader, &automatedToolingLimit,
		&vpnRequired, &vpnIPs, &secured, &md.SafeHarbor,
		&md.AccountAccess, &canCreateTest,
		&reportsCount, &totalPayout, &md.TotalPayoutCurrency, &avgReward,
		&avgFirstResponse, &scopesCount, &tags,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	md.IsPublic = nullIntToBool(isPublic)
	md.IsBounty = nullIntToBool(isBounty)
	md.IsVDP = nullIntToBool(isVDP)
	md.IsDisabled = nullIntToBool(isDisabled)
	md.CanCreateTestAccount = nullIntToBool(canCreateTest)
	md.VPNRequired = nullIntToBool(vpnRequired)
	md.Secured = nullIntToBool(secured)
	md.BountyRewardMin = nullIntToInt(bountyMin)
	md.BountyRewardMax = nullIntToInt(bountyMax)
	md.ReportsCount = nullIntToInt(reportsCount)
	md.TotalPayout = nullIntToInt(totalPayout)
	md.AvgReward = nullIntToInt(avgReward)
	md.AutomatedToolingLimit = nullIntToInt(automatedToolingLimit)
	md.ScopesCount = nullIntToInt(scopesCount)
	if avgFirstResponse.Valid {
		v := avgFirstResponse.Float64
		md.AvgFirstResponseDays = &v
	}

	if len(rewardGrids) > 0 && string(rewardGrids) != "null" {
		_ = json.Unmarshal(rewardGrids, &md.RewardGrids)
	}
	if len(qualVulns) > 0 && string(qualVulns) != "null" {
		_ = json.Unmarshal(qualVulns, &md.QualifyingVulnerabilities)
	}
	if len(nonQualVulns) > 0 && string(nonQualVulns) != "null" {
		_ = json.Unmarshal(nonQualVulns, &md.NonQualifyingVulnerabilities)
	}
	if len(oosSummary) > 0 && string(oosSummary) != "null" {
		_ = json.Unmarshal(oosSummary, &md.OutOfScopeSummary)
	}
	if len(vpnIPs) > 0 && string(vpnIPs) != "null" {
		_ = json.Unmarshal(vpnIPs, &md.VNPIPs)
	}
	if len(tags) > 0 && string(tags) != "null" {
		_ = json.Unmarshal(tags, &md.Tags)
	}
	return &md, nil
}

// boolToIntPtr converts a *bool to an interface{} suitable for SQL: nil → NULL,
// true → 1, false → 0.
func boolToIntPtr(b *bool) interface{} {
	if b == nil {
		return nil
	}
	if *b {
		return 1
	}
	return 0
}

func nullIntToBool(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	v := n.Int64 == 1
	return &v
}

func nullIntToInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// silence unused import warning for strings (used in future expansion)
var _ = strings.TrimSpace
