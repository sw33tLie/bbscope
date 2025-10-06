package yeswehack

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/sw33tLie/bbscope/v2/pkg/otp"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
	"github.com/tidwall/gjson"
)

type Poller struct{ token string }

func NewPoller(token string) *Poller { return &Poller{token: token} }

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
					handles = append(handles, allCompanySlugs[i].Str)
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

	chunkData := gjson.GetMany(res.BodyString, "scopes.#.scope", "scopes.#.scope_type", "out_of_scope")

	// Get the list of categories to filter by.
	// If nil, we'll include all categories.
	selectedCategories := scope.GetAllStringsForCategories(opts.Categories)

	for i := 0; i < len(chunkData[0].Array()); i++ {
		scopeType := chunkData[1].Array()[i].Str
		target := chunkData[0].Array()[i].Str

		// If selectedCategories is nil, it means "all" were selected, so we don't filter.
		if selectedCategories == nil {
			pData.InScope = append(pData.InScope, scope.ScopeElement{
				Target:   target,
				Category: scopeType,
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
				Target:   target,
				Category: scopeType,
			})
		}
	}

	// Handle out of scope
	outOfScopeItems := chunkData[2].Array()
	for _, item := range outOfScopeItems {
		pData.OutOfScope = append(pData.OutOfScope, scope.ScopeElement{
			Target:   item.String(),
			Category: "other",
		})
	}

	return pData, nil
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
