package dev

import (
	"context"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// This is used for testing the changes tracking db logic. Ignore

type Poller struct{}

func NewPoller() *Poller { return &Poller{} }

func (p *Poller) Name() string { return "dev" }

// Authenticate is a no-op for the dev platform.
func (p *Poller) Authenticate(ctx context.Context, cfg platforms.AuthConfig) error { return nil }

func (p *Poller) ListProgramHandles(ctx context.Context, opts platforms.PollOptions) ([]string, error) {
	// Hardcoded program URLs as handles for testing
	return []string{
		"https://example.com/program/a",
		"https://example.com/program/b",
		"https://example.com/program/c",
	}, nil
}

func (p *Poller) FetchProgramScope(ctx context.Context, handle string, opts platforms.PollOptions) (scope.ProgramData, error) {
	// Produce deterministic variations based on opts.TestRun
	pd := scope.ProgramData{Url: handle}
	switch {
	case strings.Contains(handle, "/a"):

		pd.InScope = []scope.ScopeElement{
			{Target: "https://loslo.example.com/app", Description: "app v2", Category: "website"},
			{Target: "new.a.example.com", Description: "new host", Category: "website"},
		}

		pd.OutOfScope = []scope.ScopeElement{
			{Target: "https://a.example.com/app", Description: "app", Category: "website"},
			{Target: "*.b.example.com", Description: "wildcard", Category: "website"},
		}

	case strings.Contains(handle, "/b"):
		pd.InScope = []scope.ScopeElement{{Target: "api.b.example.com", Description: "api", Category: "api"}}

	case strings.Contains(handle, "/c"):
		pd.InScope = []scope.ScopeElement{{Target: "asdasd.google.com", Description: "api", Category: "api"}}

	}
	return pd, nil
}
