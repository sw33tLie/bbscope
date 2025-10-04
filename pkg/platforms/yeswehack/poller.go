package yeswehack

import (
	"context"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
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
	// Fetch all slugs with paging using existing function
	progs := GetAllProgramsScope(p.token, false, false, "all", "t", " ", false)
	handles := make([]string, 0, len(progs))
	for _, pd := range progs {
		handles = append(handles, pd.Url)
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	// handle is https://api.yeswehack.com/programs/<slug>
	// Extract slug and call GetProgramScope
	slug := handle[strings.LastIndex(handle, "/")+1:]
	pd := GetProgramScope(p.token, slug, "all")
	return pd, nil
}
