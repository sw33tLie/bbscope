# Platforms Package

```go
import "github.com/sw33tLie/bbscope/v2/pkg/platforms"
```

The `platforms` package defines the interface that all platform pollers implement, plus shared types.

## PlatformPoller interface

```go
type PlatformPoller interface {
    Name() string
    Authenticate(ctx context.Context, cfg AuthConfig) error
    ListProgramHandles(ctx context.Context, opts PollOptions) ([]string, error)
    FetchProgramScope(ctx context.Context, handle string, opts PollOptions) (scope.ProgramData, error)
}
```

| Method | Description |
|--------|-------------|
| `Name()` | Returns the platform identifier (`h1`, `bc`, `it`, `ywh`, `immunefi`) |
| `Authenticate()` | Performs platform-specific login. Some platforms (H1, Immunefi) are no-ops |
| `ListProgramHandles()` | Returns all program handles (identifiers) matching the filter options |
| `FetchProgramScope()` | Fetches full scope data for a single program |

## Types

```go
type AuthConfig struct {
    Username  string
    Email     string
    Password  string
    Token     string
    OtpSecret string
    Proxy     string
}

type PollOptions struct {
    BountyOnly  bool
    PrivateOnly bool
    Categories  string  // comma-separated or "all"
}
```

## Platform implementations

Import the specific platform package:

```go
import (
    h1  "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
    bc  "github.com/sw33tLie/bbscope/v2/pkg/platforms/bugcrowd"
    it  "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
    ywh "github.com/sw33tLie/bbscope/v2/pkg/platforms/yeswehack"
    imm "github.com/sw33tLie/bbscope/v2/pkg/platforms/immunefi"
)
```

### Creating pollers

```go
// HackerOne — credentials passed at construction
poller := h1.NewPoller("username", "api_token")

// Bugcrowd — authenticate after construction
poller := &bc.Poller{}
err := poller.Authenticate(ctx, platforms.AuthConfig{
    Email: "...", Password: "...", OtpSecret: "...",
})

// Bugcrowd public-only (no auth)
poller := bc.NewPollerPublicOnly()

// Intigriti — authenticate with token
poller := it.NewPoller()
err := poller.Authenticate(ctx, platforms.AuthConfig{Token: "..."})

// YesWeHack — authenticate after construction
poller := &ywh.Poller{}
err := poller.Authenticate(ctx, platforms.AuthConfig{
    Email: "...", Password: "...", OtpSecret: "...",
})

// Immunefi — no auth needed
poller := imm.NewPoller()
```

## Example: fetch scope from multiple platforms

```go
ctx := context.Background()

pollers := []platforms.PlatformPoller{
    h1.NewPoller("user", "token"),
    imm.NewPoller(),
}

opts := platforms.PollOptions{BountyOnly: true}

for _, p := range pollers {
    handles, _ := p.ListProgramHandles(ctx, opts)
    for _, h := range handles {
        pd, _ := p.FetchProgramScope(ctx, h, opts)
        fmt.Printf("%s: %d in-scope targets\n", pd.Url, len(pd.InScope))
    }
}
```
