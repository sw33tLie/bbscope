# bbscope

**bbscope** is a tool for bug bounty hunters that aggregates scope data from multiple bug bounty platforms into a single, queryable interface. It supports HackerOne, Bugcrowd, Intigriti, YesWeHack, and Immunefi.

## What it does

- **Polls** all major bug bounty platforms and fetches program scopes
- **Tracks changes** — new targets, removed targets, scope updates — across poll cycles
- **Stores** everything in PostgreSQL for querying, searching, and diffing
- **Normalizes** messy scope entries using AI (optional, OpenAI-compatible)
- **Serves** a web UI with search, filtering, stats, and a REST API
- **Extracts** specific target types (wildcards, domains, URLs, IPs, CIDRs) ready for recon tools
- **Works as a Go library** — import `pkg/polling`, `pkg/platforms`, or `pkg/storage` into your own tools

## Modes of operation

| Mode | Command | Description |
|------|---------|-------------|
| **Print** | `bbscope poll` | Fetch and print scopes to stdout |
| **Database** | `bbscope poll --db` | Fetch, store, and print changes |
| **Web** | `bbscope serve` | Full web UI + REST API + background polling |

## Quick example

```bash
# Print all HackerOne in-scope targets
bbscope poll h1 --user your_user --token your_token

# Poll all platforms, store in DB, print changes
bbscope poll --db

# Extract wildcards for subdomain enumeration
bbscope db get wildcards

# Start the web interface
bbscope serve
```
