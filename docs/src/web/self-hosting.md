# Self-Hosting

The web interface is started with `bbscope serve`. It provides a full UI for browsing scopes, viewing changes, and querying targets, plus a REST API.

## Local development

```bash
bbscope serve --dev --listen :8080
```

The `--dev` flag enables development mode (e.g., no caching).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `:8080` | Address to listen on |
| `--dev` | `false` | Development mode |
| `--poll-interval` | `6` | Hours between background poll cycles |
| `--domain` | `localhost` | Domain for canonical URLs and sitemap |

## Environment variables

The web server reads platform credentials from environment variables:

| Variable | Description |
|----------|-------------|
| `DB_URL` | PostgreSQL connection string |
| `DOMAIN` | Public domain name |
| `POLL_INTERVAL` | Hours between poll cycles |
| `H1_USERNAME` | HackerOne username |
| `H1_TOKEN` | HackerOne API token |
| `BC_EMAIL` | Bugcrowd email |
| `BC_PASSWORD` | Bugcrowd password |
| `BC_OTP` | Bugcrowd TOTP secret |
| `BC_PUBLIC_ONLY` | Set to any value for Bugcrowd public-only mode |
| `IT_TOKEN` | Intigriti token |
| `YWH_EMAIL` | YesWeHack email |
| `YWH_PASSWORD` | YesWeHack password |
| `YWH_OTP` | YesWeHack TOTP secret |
| `OPENAI_API_KEY` | OpenAI API key for AI normalization |
| `OPENAI_MODEL` | Model name (default: `gpt-4.1-mini`) |

## Pages

| Path | Description |
|------|-------------|
| `/` | Landing page |
| `/programs` | Paginated program listing with search and filters |
| `/program/{platform}/{handle}` | Program detail: scope tables, recon links, change timeline |
| `/updates` | Scope changes feed |
| `/stats` | Charts: programs by platform, assets by type |
| `/api` | Interactive API explorer |
| `/docs` | Built-in feature guide |
| `/debug` | Server uptime, AI status, poller status per platform |
| `/sitemap.xml` | Auto-generated sitemap |

## Debug endpoint

The `/debug` page shows:

- Server uptime
- AI normalization status (enabled/disabled)
- Total target count
- Per-platform poller status: last run time, duration, success/failure/skipped
