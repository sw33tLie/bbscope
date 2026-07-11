package yeswehack

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/otp"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

type Poller struct {
	token  string
	bbpSet map[string]bool // tracks which handles are bounty programs
}

func NewPoller(token string) *Poller {
	return &Poller{token: token, bbpSet: map[string]bool{}}
}

func (p *Poller) Name() string { return "ywh" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Token != "" {
		p.token = cfg.Token
		return nil
	}
	if cfg.Email != "" && cfg.Password != "" && cfg.OtpSecret != "" {
		tok, err := login(cfg.Email, cfg.Password, cfg.OtpSecret, cfg.Proxy)
		if err != nil {
			return err
		}
		p.token = tok
		return nil
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	p.bbpSet = map[string]bool{}
	var handles []string
	var page = 1
	var nb_pages = 2 // Init with a value > page

	for page <= nb_pages {
		res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method:  "GET",
			URL:     "https://api.yeswehack.com/programs" + "?page=" + strconv.Itoa(page),
			Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
		}, nil)

		if err != nil {
			return nil, err
		}

		data := gjson.GetMany(res.BodyString, "items.#.slug", "items.#.bounty", "items.#.public", "items.#.disabled")
		allCompanySlugs := data[0].Array()
		allRewarding := data[1].Array()
		allPublic := data[2].Array()
		allDisabled := data[3].Array()

		for i := 0; i < len(allCompanySlugs); i++ {
			if allDisabled[i].Bool() {
				continue
			}
			if !opts.PrivateOnly || (opts.PrivateOnly && !allPublic[i].Bool()) {
				if !opts.BountyOnly || (opts.BountyOnly && allRewarding[i].Bool()) {
					slug := allCompanySlugs[i].Str
					handles = append(handles, slug)
					if allRewarding[i].Bool() {
						p.bbpSet[slug] = true
					}
				}
			}
		}

		nb_pages = int(gjson.Get(res.BodyString, "pagination.nb_pages").Int())
		page++
	}

	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	programAPIURL := "https://api.yeswehack.com/programs/" + handle
	programWebURL := "https://yeswehack.com/programs/" + handle
	pData := scope.ProgramData{Url: programWebURL}

	res, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method:  "GET",
		URL:     programAPIURL,
		Headers: []whttp.WHTTPHeader{{Name: "Authorization", Value: "Bearer " + p.token}},
	}, nil)

	if err != nil {
		return pData, err
	}

	chunkData := gjson.GetMany(res.BodyString, "scopes.#.scope", "scopes.#.scope_type", "scopes.#.asset_value", "out_of_scope")

	// Get the list of categories to filter by.
	// If nil, we'll include all categories.
	selectedCategories := scope.GetAllStringsForCategories(opts.Categories)

	isBBP := p.bbpSet[handle]

	for i := 0; i < len(chunkData[0].Array()); i++ {
		scopeType := chunkData[1].Array()[i].Str
		target := chunkData[0].Array()[i].Str

		// If selectedCategories is nil, it means "all" were selected, so we don't filter.
		if selectedCategories == nil {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:     target,
				Category:   scopeType,
				IsBBP:      isBBP,
				AssetValue: strings.ToLower(chunkData[2].Array()[i].Str),
			})
			continue
		}

		// Otherwise, check if the scopeType from the API is in our list of selected categories.
		catMatches := false
		for _, cat := range selectedCategories {
			if cat == scopeType {
				catMatches = true
				break
			}
		}

		if catMatches {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:     target,
				Category:   scopeType,
				IsBBP:      isBBP,
				AssetValue: strings.ToLower(chunkData[2].Array()[i].Str),
			})
		}
	}

	// Handle out of scope
	outOfScopeItems := chunkData[3].Array()
	for _, item := range outOfScopeItems {
		pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
			Target:   item.String(),
			Category: "other",
			IsBBP:    isBBP,
		})
	}

	pData.Metadata = extractYWHMetadata(res.BodyString, isBBP)

	return pData, nil
}

// extractYWHMetadata parses a YesWeHack program API response body and returns
// the program-level metadata. It is defensive: missing or zero-valued fields are
// left nil rather than producing zero pointers. The result is never nil.
func extractYWHMetadata(body string, isBBP bool) *scope.ProgramMetadata {
	m := &scope.ProgramMetadata{}

	// 1. Classification & Context
	m.Title = gjson.Get(body, "title").String()
	m.ProgramType = gjson.Get(body, "type").String()
	if v := gjson.Get(body, "public"); v.Exists() {
		m.IsPublic = boolPtr(v.Bool())
	}
	if v := gjson.Get(body, "bounty"); v.Exists() {
		m.IsBounty = boolPtr(v.Bool())
	}
	if v := gjson.Get(body, "vdp"); v.Exists() {
		m.IsVDP = boolPtr(v.Bool())
	}
	if v := gjson.Get(body, "disabled"); v.Exists() {
		m.IsDisabled = boolPtr(v.Bool())
	}
	if v := gjson.Get(body, "secured"); v.Exists() {
		m.Secured = boolPtr(v.Bool())
	}

	// 2. Rewards
	m.Currency = gjson.Get(body, "business_unit.currency").String()
	if v := gjson.Get(body, "bounty_reward_min").Int(); v > 0 {
		m.BountyRewardMin = intPtr(int(v))
	}
	if v := gjson.Get(body, "bounty_reward_max").Int(); v > 0 {
		m.BountyRewardMax = intPtr(int(v))
	}

	// Reward grids. YWH exposes one object per asset value (very_low, low,
	// default, medium, high, critical), each with a single bounty per severity.
	// We normalize min=max=the value and skip grids where all 4 severities are 0.
	ywhGrids := []struct {
		dimension string
		path      string
	}{
		{"very_low", "reward_grid_very_low"},
		{"low", "reward_grid_low"},
		{"default", "reward_grid_default"},
		{"medium", "reward_grid_medium"},
		{"high", "reward_grid_high"},
		{"critical", "reward_grid_critical"},
	}
	for _, g := range ywhGrids {
		grid := gjson.Get(body, g.path)
		if !grid.Exists() {
			continue
		}
		low := int(grid.Get("bounty_low").Int())
		medium := int(grid.Get("bounty_medium").Int())
		high := int(grid.Get("bounty_high").Int())
		critical := int(grid.Get("bounty_critical").Int())
		if low == 0 && medium == 0 && high == 0 && critical == 0 {
			continue
		}
		rg := scope.RewardGrid{Dimension: g.dimension}
		if low != 0 {
			rg.BountyLowMin = intPtr(low)
			rg.BountyLowMax = intPtr(low)
		}
		if medium != 0 {
			rg.BountyMediumMin = intPtr(medium)
			rg.BountyMediumMax = intPtr(medium)
		}
		if high != 0 {
			rg.BountyHighMin = intPtr(high)
			rg.BountyHighMax = intPtr(high)
		}
		if critical != 0 {
			rg.BountyCriticalMin = intPtr(critical)
			rg.BountyCriticalMax = intPtr(critical)
		}
		m.RewardGrids = append(m.RewardGrids, rg)
	}

	// 3. Scope Rules
	if v := gjson.Get(body, "rules"); v.Exists() {
		m.Rules = v.String()
		m.RulesFormat = "markdown"
	}
	m.QualifyingVulnerabilities = gjsonSlice(body, "qualifying_vulnerability")
	m.NonQualifyingVulnerabilities = gjsonSlice(body, "non_qualifying_vulnerability")
	m.OutOfScopeSummary = gjsonSlice(body, "out_of_scope")

	// 4. Testing Instructions
	m.UserAgent = gjson.Get(body, "user_agent").String()
	if v := gjson.Get(body, "vpn_active"); v.Exists() {
		m.VPNRequired = boolPtr(v.Bool())
	}
	m.VNPIPs = gjsonSlice(body, "vpn_ips")

	// 5. Account Setup
	m.AccountAccess = gjson.Get(body, "account_access").String()
	if m.AccountAccess != "" {
		aaLower := strings.ToLower(m.AccountAccess)
		if strings.Contains(aaLower, "test account") ||
			strings.Contains(aaLower, "yeswehack.ninja") ||
			strings.Contains(aaLower, "registration") {
			m.CanCreateTestAccount = boolPtr(true)
		}
	}

	// 6. Program Stats
	if v := gjson.Get(body, "scopes_count").Int(); v > 0 {
		m.ScopesCount = intPtr(int(v))
	}
	reportsCount := int(gjson.Get(body, "reports_count").Int())
	if reportsCount == 0 {
		// Fall back to stats.total_reports when the top-level field is missing/zero.
		reportsCount = int(gjson.Get(body, "stats.total_reports").Int())
	}
	if reportsCount > 0 {
		m.ReportsCount = intPtr(reportsCount)
	}
	if v := gjson.Get(body, "stats.average_reward").Int(); v > 0 {
		m.AvgReward = intPtr(int(v))
	}
	if v := gjson.Get(body, "stats.average_first_time_response"); v.Exists() {
		f := v.Float()
		m.AvgFirstResponseDays = &f
	}

	// Tags
	if tags := gjsonSlice(body, "tags"); len(tags) > 0 {
		m.Tags = tags
	}

	// Tagline / CompanyName / Industry are not exposed by the YWH program
	// endpoint in the fields we model here; leave them empty.

	return m
}

// intPtr returns a pointer to v. Local helper for building metadata, mirroring
// the unexported scope.intPtr helper.
func intPtr(v int) *int { return &v }

// boolPtr returns a pointer to v. Local helper for building metadata, mirroring
// the unexported scope.boolPtr helper.
func boolPtr(v bool) *bool { return &v }

// gjsonSlice returns the string elements of the array at path in body, or nil
// if the path does not exist or is not an array.
func gjsonSlice(body, path string) []string {
	arr := gjson.Get(body, path).Array()
	if len(arr) == 0 {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, a := range arr {
		out = append(out, a.String())
	}
	return out
}

func login(email string, password, otpSecret, proxy string) (string, error) {
	if proxy != "" {
		whttp.SetupProxy(proxy)
	}

	loginURL := "https://api.yeswehack.com/login"
	loginPayload := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	loginRes, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
		Method: "POST",
		URL:    loginURL,
		Headers: []whttp.WHTTPHeader{
			{Name: "Content-Type", Value: "application/json"},
		},
		Body: loginPayload,
	}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to send login request: %v", err)
	}

	if loginRes.StatusCode != 200 {
		return "", fmt.Errorf("login failed with status code: %d", loginRes.StatusCode)
	}

	if directToken := gjson.Get(loginRes.BodyString, "token").String(); directToken != "" {
		return directToken, nil
	}

	totpToken := gjson.Get(loginRes.BodyString, "totp_token").String()
	if totpToken == "" {
		return "", fmt.Errorf("invalid login response: neither token nor totp_token found")
	}

	if otpSecret == "" {
		return "", fmt.Errorf("2FA is enabled but no OTP secret provided")
	}

	OTP_ATTEMPTS := 5
	for attempts := 1; attempts <= OTP_ATTEMPTS; attempts++ {
		code, err := otp.GenerateTOTP(otpSecret, time.Now())
		if err != nil {
			return "", fmt.Errorf("failed to generate TOTP: %v", err)
		}

		totpURL := "https://api.yeswehack.com/account/totp"
		totpPayload := fmt.Sprintf(`{"token":"%s","code":"%s"}`, totpToken, code)

		totpRes, err := whttp.SendHTTPRequest(&whttp.WHTTPReq{
			Method: "POST",
			URL:    totpURL,
			Headers: []whttp.WHTTPHeader{
				{Name: "Content-Type", Value: "application/json"},
			},
			Body: totpPayload,
		}, nil)

		if err != nil {
			return "", fmt.Errorf("failed to send TOTP request: %v", err)
		}

		if totpRes.StatusCode != 400 {
			if totpRes.StatusCode != 200 {
				return "", fmt.Errorf("TOTP verification failed with status code: %d", totpRes.StatusCode)
			}
			finalToken := gjson.Get(totpRes.BodyString, "token").String()
			if finalToken == "" {
				return "", fmt.Errorf("final token not found in TOTP response")
			}
			return finalToken, nil
		}

		time.Sleep(2 * time.Second)
		if attempts == OTP_ATTEMPTS {
			return "", fmt.Errorf("TOTP verification failed after %d attempts", OTP_ATTEMPTS)
		}
	}

	return "", fmt.Errorf("unexpected error in TOTP verification")
}
