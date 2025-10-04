# plan for bbscope v2 refactor

## Points to consider:
- We're going to break backward compatibility with subcommands. We WILL deploy on github.com/sw33tLie/bbscope/v2 to avoid breaking users and pipelines.
- New subcommands `bbscope (poll|db|??)`

## bbscope poll

- `--platform` flag (allow multiple comma-separated values, or "all" for all configured in `~/.bbscope.yaml`)

Subcommands of the `poll` subcommand

## bbscope get 
- all
- URLs
- wildcards


Subcommands or maybe a flag for type? This prints scope by type from the DB if the DB exists.

## What DB to use

- Use SQLite
- Use sqlc to generate sql schema and funcs automatically
- I want change monitoring. That might be easier after loading everything in RAM as JSON, but I’m not sure I should store JSON; maybe SQLite is better. Changes can be items added, removed, or modified. Once an item is deleted, I want to save it as deleted but still keep it in my DB so I can track when it was deleted. Insertions should have timestamps as well.
- I want good error handling.
- If a scope poll run fails, I don't want it to pollute my DB with broken records.

## Other things to consider
- I want to keep the same code structure for all `poll` subcommands across different platforms. Should I use interfaces? The flow is always the same: first get all program handles, then fetch scope for each, etc.
- Keep my same HTTP library `whttp`.
- Make sure the tool can still work as a Go library (exported/uppercase functions, easy to use).
- Config YAML file at `~/.bbscope.yaml` with auth tokens for each platform. For `poll` commands, make it possible to override YAML creds via CLI flags.
- Keep the same output flags for formatting scope (URL, target, category, etc.).
- The `poll` command should have an optional `--db` flag: with it, store scope in the DB; otherwise, just print. When storing in the DB, print only changes (using the change‑monitoring algorithm); without it, print everything found.

## MVP sqlite schema (v2)

Goals:
- Identify a program by its full URL (platform base + handle).
- Maintain a single current-state table with first/last seen timestamps.
- Keep a lightweight changes log (append-only) for diffs.
- Use one transaction per program to avoid partial writes.

Notes:
- Prefer the `modernc.org/sqlite` driver to keep `go install` simple (no CGO).
- Enable WAL mode at startup; migrations optional at first.

### Tables

```sql
-- Current state only
CREATE TABLE scope_entries (
  id                INTEGER PRIMARY KEY,
  program_url       TEXT NOT NULL,          -- full canonical URL
  platform          TEXT NOT NULL,          -- h1 | bc | it | ywh | immunefi
  handle            TEXT NOT NULL,          -- convenient filter
  target_normalized TEXT NOT NULL,
  target_raw        TEXT,
  category          TEXT NOT NULL,
  description       TEXT,
  in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
  first_seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(program_url, target_normalized, category, in_scope)
);

CREATE INDEX idx_scope_program ON scope_entries(program_url);
CREATE INDEX idx_scope_identity ON scope_entries(program_url, target_normalized, category, in_scope);

-- minimal changes history (append-only)
CREATE TABLE scope_changes (
  id                INTEGER PRIMARY KEY,
  occurred_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  program_url       TEXT NOT NULL,
  platform          TEXT NOT NULL,
  handle            TEXT NOT NULL,
  target_normalized TEXT NOT NULL,
  category          TEXT NOT NULL,
  in_scope          INTEGER NOT NULL CHECK (in_scope IN (0,1)),
  change_type       TEXT NOT NULL CHECK (change_type IN ('added','updated','removed'))
);

CREATE INDEX idx_changes_time ON scope_changes(occurred_at);
CREATE INDEX idx_changes_program ON scope_changes(program_url, occurred_at);
```

### Ingestion rules
- For each program: fetch all scope, normalize to `target_normalized`, then compute diff vs `scope_entries` in-memory.
- Wrap each program update in a single transaction:
  - Upsert new/updated rows in `scope_entries`, updating `last_seen_at` (and `first_seen_at` on insert).
  - Delete rows that disappeared from the latest fetch for that program.
  - Insert corresponding rows into `scope_changes` for `added`, `updated`, and `removed` (optional).
- If the program fetch fails, roll back the transaction so the DB is not polluted.

### Normalization (for `target_normalized`)
- Lowercase hostnames; strip surrounding whitespace.
- Remove default ports (80/443) and trailing slashes.
- Canonicalize wildcards to a standard form like `*.example.com`.
- For URL targets, store a canonical form; for wildcard/domain targets, store the hostname/wildcard.
- Treat empty `uri` as `name` (as v1 does) before normalization.

### Querying examples (from DB)
- Current in-scope URLs: select from `scope_entries` where `in_scope = 1` and `category IN ('website','api')`.
- Wildcards only: filter `target_normalized` that begins with `*.`.
- Changes since a time: select from `scope_changes` where `occurred_at >= ?`.

- If a run fails, I DO NOT want half-written polls/updates; keep it as it was before.
- The `--platform` flag for the `poll` command should support "all" or multiple platforms, comma-separated, not just one at a time.
- I want optional Dockerfile support.
- SQLite should work with `go install` and not require any weird manual configuration.