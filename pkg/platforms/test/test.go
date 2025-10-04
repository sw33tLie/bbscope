package test

import (
	"context"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

type Poller struct{}

func (p *Poller) Name() string { return "test" }

// Authenticate is a no-op for the test platform.
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	// Hardcoded program URLs as handles for testing
	return []string{
		"https://example.com/program/a",
		"https://example.com/program/b",
	}, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	// Produce deterministic variations based on opts.TestRun
	pd := scope.ProgramData{Url: handle}
	switch {
	case strings.Contains(handle, "/a"):

		pd.InScope = []scope.ScopeElement{
			{Target: "https://lossslo.example.com/app", Description: "app v2", Category: "website"},
			{Target: "*.a.example.com", Description: "wildcard", Category: "website"},
			{Target: "new.a.example.com", Description: "new host", Category: "website"},
		}

		pd.OutOfScope = []scope.ScopeElement{
			{Target: "https://a.example.com/app", Description: "app", Category: "website"},
			{Target: "*.a.example.com", Description: "wildcard", Category: "website"},
		}

	case strings.Contains(handle, "/b"):
		pd.InScope = []scope.ScopeElement{{Target: "api.b.example.com", Description: "api", Category: "api"}}
	}
	return pd, nil
}
