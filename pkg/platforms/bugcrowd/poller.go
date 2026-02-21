package bugcrowd

import (
	"context"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

type Poller struct {
	token    string
	bbpSet   map[string]bool // tracks which handles are bug_bounty programs
}

// NewPollerFromToken uses an existing _bugcrowd_session token.
func NewPollerFromToken(token string) *Poller {
	return &Poller{token: token, bbpSet: map[string]bool{}}
}

// NewPollerWithLogin logs in using email/password and OTP secret to obtain a session token.
func NewPollerWithLogin(email, password, otpSecret, proxy string) (*Poller, error) {
	tok, err := Login(email, password, otpSecret, proxy)
	if err != nil {
		return nil, err
	}
	return &Poller{token: tok, bbpSet: map[string]bool{}}, nil
}

func (p *Poller) Name() string { return "bc" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Token != "" {
		p.token = cfg.Token
		return nil
	}
	if cfg.Email != "" && cfg.Password != "" && cfg.OtpSecret != "" {
		tok, err := Login(cfg.Email, cfg.Password, cfg.OtpSecret, cfg.Proxy)
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

	// Reuse existing listing: bbpOnly controls category, pvtOnly controls open/private
	handles, err := GetProgramHandles(p.token, "bug_bounty", opts.PrivateOnly)
	if err != nil {
		return nil, err
	}
	for _, h := range handles {
		p.bbpSet[h] = true
	}

	// Optionally include VDP if not bbpOnly
	if !opts.BountyOnly {
		vdp, err := GetProgramHandles(p.token, "vdp", opts.PrivateOnly)
		if err == nil {
			handles = append(handles, vdp...)
		}
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	cats := "all"
	pd, err := GetProgramScope(handle, cats, p.token)
	if err != nil {
		return scope.ProgramData{Url: strings.TrimPrefix(handle, "/")}, err
	}

	// Set IsBBP on all targets based on whether the handle was listed as bug_bounty
	isBBP := p.bbpSet[handle]
	if isBBP {
		for i := range pd.InScope {
			pd.InScope[i].IsBBP = true
		}
		for i := range pd.OutOfScope {
			pd.OutOfScope[i].IsBBP = true
		}
	}

	return pd, nil
}
