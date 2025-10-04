package platforms

import (
	"context"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// PollOptions carries optional controls/filters used by pollers.
type PollOptions struct {
	PrivateOnly   bool
	BountyOnly    bool
	Categories    string
	ProgramFilter string // handle or full URL substring
	TestRun       string // for the test platform to vary outputs deterministically
}

// AuthConfig carries optional authentication inputs.
type AuthConfig struct {
	Username  string
	Email     string
	Password  string
	Token     string
	OtpSecret string
	Proxy     string
}

// PlatformPoller is a minimal interface for listing programs and fetching scope.
type PlatformPoller interface {
	Name() string
	// Authenticate configures the poller with credentials, if needed.
	// Implementations that don't require auth should return nil.
	Authenticate(ctx context.Context, cfg AuthConfig) error
	ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
	FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}
