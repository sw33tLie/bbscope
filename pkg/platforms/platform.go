package platforms

import (
	"context"

	"github.com/sw33tLie/bbscope/v2/pkg/scope"
)

// PollOptions carries optional controls/filters used by pollers.
type PollOptions struct {
	BountyOnly  bool
	PrivateOnly bool
	Categories  string
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

// PlatformPoller defines a common interface for platform-specific polling operations,
// abstracting away the details of authentication, program discovery, and scope fetching.
type PlatformPoller interface {
	Name() string
	// Authenticate configures the poller with credentials, if needed.
	// Implementations that don't require auth should return nil.
	Authenticate(ctx context.Context, cfg AuthConfig) error
	ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
	FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}
