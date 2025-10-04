package hackerone

import (
	"context"
	"encoding/base64"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// Poller adapts existing H1 code to the generic PlatformPoller interface.
type Poller struct {
	authB64 string
}

// NewPoller builds a HackerOne poller from username and API token.
func NewPoller(username, token string) *Poller {
	// Existing code expects base64(username:token) without the "Basic " prefix.
	raw := username + ":" + token
	return &Poller{authB64: base64.StdEncoding.EncodeToString([]byte(raw))}
}

func (p *Poller) Name() string { return "h1" }

func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error {
	if cfg.Username != "" && cfg.Token != "" {
		raw := cfg.Username + ":" + cfg.Token
		p.authB64 = base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return nil
}

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	// Map generic options to H1 parameters
	pvtOnly := opts.PrivateOnly
	publicOnly := false
	active := true
	bbpOnly := opts.BountyOnly
	handles := getProgramHandles(p.authB64, pvtOnly, publicOnly, active, bbpOnly)
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	cats := getCategories("all")
	includeOOS := false
	return getProgramScope(p.authB64, handle, opts.BountyOnly, cats, includeOOS)
}
