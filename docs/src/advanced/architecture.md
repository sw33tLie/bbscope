# Architecture

## Project structure

```
bbscope/
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go             # Root command, global flags
│   ├── poll.go             # bbscope poll
│   ├── poll_*.go           # Platform-specific poll subcommands
│   ├── db.go               # bbscope db
│   ├── db_*.go             # DB subcommands (stats, changes, find, etc.)
│   ├── serve.go            # bbscope serve
│   └── dev.go              # bbscope dev
├── pkg/                    # Library packages (importable)
│   ├── platforms/          # Platform interface + implementations
│   │   ├── platform.go     # PlatformPoller interface
│   │   ├── hackerone/
│   │   ├── bugcrowd/
│   │   ├── intigriti/
│   │   ├── yeswehack/
│   │   ├── immunefi/
│   │   └── dev/
│   ├── polling/            # Shared polling orchestrator
│   │   └── polling.go
│   ├── storage/            # PostgreSQL persistence
│   │   ├── storage.go      # DB operations
│   │   ├── types.go        # Entry, Change, UpsertEntry, etc.
│   │   ├── normalize.go    # Target normalization
│   │   ├── transform.go    # Aggressive transforms
│   │   └── extra.go        # Additional queries
│   ├── scope/              # Core types and category normalization
│   ├── targets/            # Target extraction (wildcards, domains, etc.)
│   ├── ai/                 # AI normalization
│   ├── whttp/              # HTTP client wrapper (retryablehttp)
│   └── otp/                # TOTP generation
├── website/                # Web server
│   ├── pkg/core/           # Server core (routes, handlers, poller)
│   ├── static/             # CSS, JS, images
│   ├── docker-compose.yml
│   └── Dockerfile
├── internal/
│   └── utils/              # Logging setup
└── docs/                   # This documentation (mdBook)
```

## Data flow

### CLI polling (`bbscope poll --db`)

```
Platform API
    ↓ ListProgramHandles()
    ↓ FetchProgramScope() × N (concurrent workers)
    ↓
pkg/polling.PollPlatform()
    ↓ AI normalize (optional, with DB cache)
    ↓ BuildEntries()
    ↓ UpsertProgramEntries() → changes
    ↓ LogChanges()
    ↓ OnProgramDone callback → printChanges()
    ↓
    ↓ SyncPlatformPrograms() → removed program changes
    ↓
stdout (change output)
```

### Web server (`bbscope serve`)

```
Background goroutine (every N hours)
    ↓ buildPollers() from env vars
    ↓ For each platform (concurrent):
    │   polling.PollPlatform()
    ↓ invalidateProgramsCache()

HTTP requests
    ↓ /api/v1/* → query DB → JSON response (cached)
    ↓ /programs, /program/* → query DB → HTML (gomponents)
```

## Key design decisions

- **Platform interface**: All platforms implement `PlatformPoller`. Adding a new platform means implementing 4 methods.
- **Shared orchestrator**: `pkg/polling` contains the polling logic used by both CLI and web server, avoiding duplication.
- **Change detection in DB**: `UpsertProgramEntries` does an atomic compare-and-update, returning only what changed.
- **Safety checks**: Multiple layers prevent accidental data loss (scope wipe detection, platform-level 0-program check).
- **AI caching**: Normalized targets are stored in `targets_ai_enhanced` so only new/changed targets hit the API.
- **Category unification**: Platform-specific category names are mapped to a unified set in `pkg/scope`.
