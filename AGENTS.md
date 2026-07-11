# AGENTS.md

Guidance for AI agents working in this repository. Read this before editing code.

## Project overview

`bbscope` is a Go CLI that aggregates bug bounty program scopes from HackerOne,
Bugcrowd, Intigriti, YesWeHack, and Immunefi, optionally stores them in
PostgreSQL, tracks changes over time, and exposes a web UI at `bbscope.com`.

- Module path: `github.com/sw33tLie/bbscope/v2`
- Go version: 1.24+ (see `go.mod`)
- Entry point: `main.go` → `cmd.Execute()`
- This is **v2**. The command structure is `poll`, `db`, `reports`, `serve`.
  Do not reintroduce the v1 top-level commands (`bbscope h1`, etc.). The
  legacy redirect in `cmd/root.go` exists only for backward compatibility.

## Repository layout

```
main.go              – entry point, just calls cmd.Execute()
cmd/                 – cobra commands (one file per command/subcommand)
internal/utils/      – private helpers (logging, net parsing). Not importable outside the module.
pkg/                 – public, reusable packages:
  ai/                – LLM-based scope normalizer (OpenAI-compatible)
  otp/               – TOTP helpers for platform logins
  platforms/         – PlatformPoller interface + per-platform implementations
    <platform>/      – hackerone, bugcrowd, intigriti, yeswehack, immunefi, dev
  polling/           – concurrent polling orchestration (DB-aware)
  reports/           – report downloading
  scope/             – scope data types and printing helpers
  storage/           – PostgreSQL layer (lib/pq) + entry/upsert/change logic
  targets/           – target extraction (wildcards, domains, urls, cidrs, ips)
  whttp/             – thin HTTP wrapper around retryablehttp
website/             – self-hosted web UI/API (separate Dockerfile, its own deps)
docs/                – mdBook documentation (deployed via .github/workflows/docs.yml)
.github/workflows/   – docker.yml builds/pushes GHCR image; docs.yml deploys docs
Dockerfile           – multi-stage alpine build of the CLI binary
```

When adding a feature, place it in the package that matches its concern:
- New platform → `pkg/platforms/<name>/poller.go` implementing `PlatformPoller`,
  plus a `cmd/poll_<name>.go` that wires it in.
- New CLI subcommand → file named after the command (e.g. `db_add.go`).
- New queryable target type → file under `pkg/targets/` + a `cmd/get_<type>.go`.

## Architecture invariants

- **`platforms.PlatformPoller` is the integration boundary.** Every platform
  implements `Name()`, `Authenticate()`, `ListProgramHandles()`, and
  `FetchProgramScope()` (see `pkg/platforms/platform.go`). Do not bypass this
  interface by calling platform HTTP code directly from `cmd/` or `polling/`.
- **`polling.PollPlatform` orchestrates a poll run.** It owns concurrency, DB
  upsert, AI normalization, change logging, and the "abort if 0 programs returned
  but DB has many" safety check. CLI code should call into it, not reimplement
  the worker pool. The only exception is `runPollNoDB` in `cmd/poll.go`, which is
  the no-DB streaming path.
- **All DB access goes through `pkg/storage`.** Never use `database/sql` or
  `lib/pq` directly from `cmd/`, `polling/`, or platform packages.
- **All outbound HTTP goes through `pkg/whttp`** (`whttp.SendHTTPRequest` with
  a `WHTTPReq`). This is what makes `--proxy` and `--debug-http` work globally.
- **Concurrency pattern**: bounded worker pool — buffered channel of handles,
  `N` goroutines, `sync.WaitGroup`, `sync.Mutex` guarding result slices.
  Default concurrency is `5` when unspecified.
- **Context propagation**: interface methods take `context.Context` as the
  first argument. Prefer `cmd.Context()` from cobra; fall back to
  `context.Background()` only at top-level entry points.
- **Don't silently wipe data.** `storage.ErrAbortingScopeWipe` and the
  "0 programs returned but DB has >10" check in `polling.go` exist to prevent
  accidental data loss. Preserve and respect both when touching poll/sync code.

## Coding conventions

- **gofmt / goimports**: run `gofmt -w` on changed files. Imports are grouped as
  stdlib, blank line, then third-party, blank line, then `github.com/sw33tLie/...`.
  See any file in `pkg/platforms/hackerone/` for the canonical grouping.
- **Exported identifiers must have doc comments** starting with the identifier
  name (Go convention). Unexported helpers don't need them.
- **Errors**: return them; do not panic. Wrap with context using
  `fmt.Errorf("...: %w", err)` when the caller is expected to inspect the error,
  otherwise a plain `fmt.Errorf` is fine. Log non-fatal errors via `utils.Log`
  and continue; only return an error when the run cannot proceed.
- **Logging**: use `utils.Log` (a `*logrus.Logger`) with leveled methods —
  `Infof`, `Warnf`, `Errorf`, `Debugf`. Level is set from `--loglevel`.
  Do **not** use `fmt.Println`/`fmt.Printf` for logs. Those are reserved for
  actual CLI stdout output that the user pipes to other tools
  (`bbscope db get wildcards | subfinder`).
- **HTTP responses**: parse with `gjson` (`github.com/tidwall/gjson`) for
  nested JSON, `goquery` for HTML scraping. Don't pull in a new JSON library.
- **Domain parsing**: use `github.com/weppos/publicsuffix-go` and helpers in
  `pkg/targets` / `internal/utils` (`IsCIDR`, `IsIP`, `IsIPRange`). Don't
  reinvent wildcard/CIDR detection.
- **Config**: read via `viper.GetString(...)` etc. Default values are set in
  `cmd/root.go` `initConfig()`. CLI flags override config via
  `viper.BindPFlag(...)`. When adding a flag, register a default in
  `initConfig` if it should also be readable from `~/.bbscope.yaml` or env.
- **Cobra command shape**: each subcommand file declares a `var fooCmd =
  &cobra.Command{...}` and registers it in `init()`. Common flags are
  `PersistentFlags` on the parent so subcommands inherit them. See
  `cmd/poll.go` for the canonical example.

## Testing conventions

- Tests live **alongside** the code: `foo.go` → `foo_test.go` in the same
  package. See `pkg/ai/normalizer_test.go`, `pkg/targets/wildcards_test.go`.
- Table-driven tests are the norm (`tests := []struct{ name string; ... }{...}`,
  `t.Run(tt.name, ...)`).
- No external test services or network in unit tests — the AI normalizer test
  injects a fake `normalizedResult` map rather than calling OpenAI. Follow that
  pattern for anything that would otherwise hit a real API or DB.
- Run tests with `go test ./...` from the repo root. Do not commit a change
  that breaks `go vet ./...` or `go build ./...`.

## Build, run, validate

```bash
# Build the CLI (matches the Dockerfile's build flags)
CGO_ENABLED=0 go build -ldflags="-w -s" -o bbscope main.go

# Run all tests + vet
go vet ./...
go test ./...

# Run the CLI locally
go run . poll h1 --user <user> --token <token>
go run . poll --db -b -p

# Run the web UI locally (needs DB_URL)
DB_URL="postgres://user:password@localhost:5432/bbscope?sslmode=disable" \
  go run *.go serve --dev --poll-interval 0

# Docs site (mdBook)
cd docs && mdbook serve   # http://localhost:3000
```

## CLI / UX conventions

- **Output the user pipes to other tools must stay on stdout and be
  machine-friendly** (one target per line, no logos, no log lines). The
  `LOGO` constant and logrus output are for stderr/the help banner only.
- **Flag style**: long flags use `--kebab-case`, short flags use a single char.
  Keep flag names stable — they're part of the public interface and are
  documented in `README.md` and `docs/`.
- **Don't break the v2 command tree** (`poll`, `db`, `reports`, `serve`).
  Adding subcommands is fine; renaming existing ones is a breaking change.
- **Authentication flags override config**, never the reverse. See how
  `cmd/poll_h1.go` binds `--user`/`--token` to `viper` keys.

## Security & credentials

- **Never hardcode API tokens, passwords, OTP secrets, or DB URLs.** They
  belong in `~/.bbscope.yaml` (gitignored) or env vars (`DB_URL`,
  `OPENAI_API_KEY`). Read them via `viper` or `os.Getenv`.
- The `~/.bbscope.yaml` template in `README.md` is the only place to show
  example credentials, and they must remain placeholders.
- Be careful with logging: do not log request/response bodies when they may
  contain auth headers or tokens. `--debug-http` is opt-in for that reason.

## Documentation

- User-facing changes (new flags, new commands, behavior changes) must be
  reflected in `README.md` **and** the relevant page under `docs/src/`.
- The docs site is built with mdBook from `docs/src/SUMMARY.md`. Rebuild
  locally with `mdbook serve docs/` before submitting docs changes.
- `website/README.md` covers the self-hosted web UI; update it when the
  web server's flags or endpoints change.

## Things to avoid

- Don't add new top-level dependencies without strong justification. Prefer
  what's already in `go.mod` (`cobra`, `viper`, `logrus`, `gjson`, `goquery`,
  `publicsuffix-go`, `lib/pq`, `retryablehttp`).
- Don't `go fmt` the entire repo in a feature PR — format only the files you
  changed, to keep diffs reviewable.
- Don't refactor working platform pollers for style reasons. They encode
  fragile, reverse-engineered API quirks (pagination edge cases, retry
  behavior, partial-data fallbacks). Preserve that logic exactly.
- Don't remove the legacy-command redirect in `cmd/root.go`; users still rely
  on `bbscope h1` working.
- Don't introduce a new JSON/HTTP/logging/config library. Use `whttp`,
  `utils.Log`, `viper`, and `gjson`.