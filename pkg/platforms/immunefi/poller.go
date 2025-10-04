package immunefi

import (
	"context"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

type Poller struct{}

func (p *Poller) Name() string { return "immunefi" }

// Authenticate is a no-op for Immunefi (no auth required)
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	// We don't have a light listing endpoint; reuse existing, then return URLs
	progs := GetAllProgramsScope("all", 8)
	handles := make([]string, 0, len(progs))
	for _, pd := range progs {
		handles = append(handles, pd.Url)
	}
	return handles, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	// The existing code scrapes each program URL; call the listing and pick the match for simplicity
	progs := GetAllProgramsScope("all", 8)
	for _, pd := range progs {
		if pd.Url == handle {
			return pd, nil
		}
	}
	return scope.ProgramData{Url: handle}, nil
}
