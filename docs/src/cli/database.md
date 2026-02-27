# Database Commands

The `bbscope db` command group provides tools for querying and managing the scope database.

All commands require a database connection, configured via `db.url` in the config file or `--db-url` flag.

## Stats

View per-platform statistics:

```bash
bbscope db stats
```

Outputs a table with program counts, in-scope targets, and out-of-scope targets per platform.

## Changes

View recent scope changes:

```bash
# Last 50 changes (default)
bbscope db changes

# Last 200 changes
bbscope db changes --limit 200
```

## Print

Dump the full scope database with filters:

```bash
# All in-scope targets
bbscope db print

# Include out-of-scope
bbscope db print --oos

# Filter by platform
bbscope db print --platform h1

# Filter by program
bbscope db print --program "https://hackerone.com/example"

# JSON output
bbscope db print --format json

# CSV output
bbscope db print --format csv

# Custom text output
bbscope db print -o tdu -d $'\t'

# Only changes since a date
bbscope db print --since 2025-01-01T00:00:00Z
```

## Find

Full-text search across current and historical scope entries:

```bash
bbscope db find "example.com"
```

Results tagged `[historical]` are targets that were previously in scope but have since been removed.

## Add custom targets

```bash
bbscope db add --target "*.custom.com" --category wildcard --program-url "https://example.com/program"
```

## Ignore / Unignore programs

Ignored programs are skipped during polling:

```bash
# Ignore a program
bbscope db ignore --program-url "https://hackerone.com/example"

# Unignore
bbscope db unignore --program-url "https://hackerone.com/example"

# Include ignored programs in print output
bbscope db print --include-ignored
```

## Shell

Open a `psql` shell connected to the bbscope database, with a schema reference printed:

```bash
bbscope db shell
```
