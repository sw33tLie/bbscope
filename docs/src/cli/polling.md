# Polling Scopes

The `bbscope poll` command fetches program scopes from bug bounty platforms.

## Basic usage

```bash
# Poll all configured platforms
bbscope poll

# Poll a specific platform
bbscope poll h1
bbscope poll bc
bbscope poll it
bbscope poll ywh
bbscope poll immunefi
```

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--db` | | `false` | Save results to PostgreSQL and print changes |
| `--ai` | | `false` | Enable AI normalization (requires `--db` and API key) |
| `--concurrency` | | `5` | Concurrent program fetches per platform |
| `--category` | | `all` | Filter by scope category (wildcard, url, cidr, etc.) |
| `--bbp-only` | `-b` | `false` | Only programs with monetary rewards |
| `--private-only` | `-p` | `false` | Only private programs |
| `--oos` | | `false` | Include out-of-scope elements |
| `--output` | `-o` | `tu` | Output flags: `t`=target, `d`=description, `c`=category, `u`=program URL |
| `--delimiter` | `-d` | `" "` | Delimiter for output fields |
| `--since` | | | Only print changes since RFC3339 timestamp (requires `--db`) |

## Platform-specific flags

Each platform subcommand accepts inline credentials, useful for one-off runs without a config file:

```bash
# HackerOne
bbscope poll h1 --user your_user --token your_token

# Bugcrowd
bbscope poll bc --email you@example.com --password pass --otp-secret SECRET

# Bugcrowd public-only (no auth)
bbscope poll bc --public-only

# Intigriti
bbscope poll it --token your_token

# YesWeHack
bbscope poll ywh --email you@example.com --password pass --otp-secret SECRET
```

## Database mode

With `--db`, bbscope tracks scope state across runs:

- **First run**: Populates the database silently (no change output).
- **Subsequent runs**: Prints only what changed since last poll.
- **Safety check**: If a platform returns 0 programs but the database has >10, the sync is aborted to prevent accidental data loss.

```bash
# First run — silent population
bbscope poll --db

# Second run — prints changes
bbscope poll --db
# 🆕  h1  https://hackerone.com/example  *.new-target.com
# ❌  bc  https://bugcrowd.com/example  removed-target.com
```

## Output formatting

The `-o` flag controls which fields are printed (non-DB mode):

```bash
# Target only
bbscope poll -o t

# Target + description + category + program URL
bbscope poll -o tdcu

# Tab-delimited for piping
bbscope poll -o tdu -d $'\t'
```

## Filtering by category

```bash
# Only wildcard targets
bbscope poll --category wildcard

# Only mobile apps
bbscope poll --category android,ios

# Multiple categories
bbscope poll --category wildcard,url,cidr
```

Available categories: `wildcard`, `url`, `cidr`, `android`, `ios`, `ai`, `hardware`, `blockchain`, `binary`, `code`, `other`.
