# Using bbscope as a Library

bbscope's packages under `pkg/` are designed to be importable by other Go projects. You can use them to build custom tools without depending on the CLI.

## Install

```bash
go get github.com/sw33tLie/bbscope/v2@latest
```

## Packages

| Package | Import | Purpose |
|---------|--------|---------|
| [`polling`](./polling.md) | `pkg/polling` | High-level orchestrator: poll a platform, upsert to DB, track changes |
| [`platforms`](./platforms.md) | `pkg/platforms` | Platform interface + implementations for H1, BC, IT, YWH, Immunefi |
| [`storage`](./storage.md) | `pkg/storage` | PostgreSQL storage layer: upsert, query, search, change tracking |
| [`targets`](./targets.md) | `pkg/targets` | Extract wildcards, domains, URLs, IPs, CIDRs from scope entries |
| `scope` | `pkg/scope` | Core types (`ProgramData`, `ScopeElement`) and category normalization |
| `ai` | `pkg/ai` | AI normalization interface and OpenAI implementation |

## Quick example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/sw33tLie/bbscope/v2/pkg/platforms"
    h1 "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
)

func main() {
    ctx := context.Background()
    poller := h1.NewPoller("your_user", "your_token")

    handles, err := poller.ListProgramHandles(ctx, platforms.PollOptions{})
    if err != nil {
        log.Fatal(err)
    }

    for _, h := range handles {
        pd, err := poller.FetchProgramScope(ctx, h, platforms.PollOptions{})
        if err != nil {
            log.Printf("error: %s: %v", h, err)
            continue
        }
        for _, s := range pd.InScope {
            fmt.Printf("%s  %s  %s\n", pd.Url, s.Category, s.Target)
        }
    }
}
```
