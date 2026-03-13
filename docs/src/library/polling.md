# Polling Package

```go
import "github.com/sw33tLie/bbscope/v2/pkg/polling"
```

The `polling` package provides the high-level orchestrator that both the CLI and web server use. It handles the full lifecycle of polling a single platform: listing programs, fetching scopes concurrently, AI normalization with DB caching, upserting to the database, and tracking changes.

## API

### PollPlatform

```go
func PollPlatform(ctx context.Context, cfg PlatformConfig) (*PlatformResult, error)
```

Polls a single platform end-to-end. The caller decides how to loop over multiple platforms (sequentially, concurrently, etc.).

### PlatformConfig

```go
type PlatformConfig struct {
    Poller      platforms.PlatformPoller  // required
    Options     platforms.PollOptions     // category/bounty/private filters
    DB          *storage.DB              // required
    Concurrency int                      // defaults to 5 if <= 0
    Normalizer  ai.Normalizer            // optional; nil = skip AI
    Log         Logger                   // optional; nil = no logging

    // Called per-program after upsert (from worker goroutines).
    // Nil = no callback.
    OnProgramDone func(programURL string, changes []storage.Change, isFirstRun bool)
}
```

### PlatformResult

```go
type PlatformResult struct {
    PolledProgramURLs     []string         // URLs of all successfully polled programs
    ProgramChanges        []storage.Change // per-program changes accumulated
    RemovedProgramChanges []storage.Change // changes from program sync (disabled programs)
    IsFirstRun            bool             // true if DB had 0 programs for this platform
    Errors                []error          // non-fatal errors (fetch failures, DB errors)
}
```

### Logger

```go
type Logger interface {
    Infof(format string, args ...interface{})
    Warnf(format string, args ...interface{})
    Errorf(format string, args ...interface{})
    Debugf(format string, args ...interface{})
}
```

Plug in any logger. Both logrus and stdlib `log.Printf` wrappers satisfy this interface.

## Example: poll HackerOne with streaming output

```go
package main

import (
    "context"
    "fmt"
    "log"

    h1 "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
    "github.com/sw33tLie/bbscope/v2/pkg/polling"
    "github.com/sw33tLie/bbscope/v2/pkg/storage"
)

type myLogger struct{}
func (myLogger) Infof(f string, a ...interface{})  { log.Printf("[INFO] "+f, a...) }
func (myLogger) Warnf(f string, a ...interface{})  { log.Printf("[WARN] "+f, a...) }
func (myLogger) Errorf(f string, a ...interface{}) { log.Printf("[ERROR] "+f, a...) }
func (myLogger) Debugf(f string, a ...interface{}) {}

func main() {
    db, _ := storage.Open("postgres://user:pass@localhost/bbscope?sslmode=disable")
    defer db.Close()

    poller := h1.NewPoller("user", "token")

    result, err := polling.PollPlatform(context.Background(), polling.PlatformConfig{
        Poller:      poller,
        DB:          db,
        Concurrency: 10,
        Log:         myLogger{},
        OnProgramDone: func(url string, changes []storage.Change, isFirstRun bool) {
            if !isFirstRun {
                for _, c := range changes {
                    fmt.Printf("[%s] %s %s\n", c.ChangeType, c.ProgramURL, c.TargetRaw)
                }
            }
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Polled %d programs, %d changes\n",
        len(result.PolledProgramURLs), len(result.ProgramChanges))
}
```

## What PollPlatform does internally

1. Checks if this is the first run (DB program count == 0)
2. Loads ignored programs from the DB
3. Calls `ListProgramHandles()` on the platform
4. Safety check: aborts if 0 handles returned but DB has >10 programs
5. Runs a worker pool (`Concurrency` goroutines) that for each program:
   - Fetches scope via `FetchProgramScope()`
   - Skips ignored programs
   - Builds `TargetItem` list from scope elements
   - Applies AI normalization (loads cached enhancements, only normalizes new items)
   - Calls `BuildEntries()` and `UpsertProgramEntries()`
   - Calls `LogChanges()` (skipped on first run)
   - Calls `OnProgramDone` callback if set
6. Calls `SyncPlatformPrograms()` to mark removed programs as disabled
7. Logs changes for removed programs (skipped on first run)
8. Returns `PlatformResult` with all accumulated data
