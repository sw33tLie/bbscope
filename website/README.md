# bbscope.com

Web interface and API for aggregating bug bounty program scopes from HackerOne, Bugcrowd, Intigriti, and YesWeHack.

## Quick start (Docker)

```bash
cd website
cp .env.example .env
# Edit .env — at minimum set POSTGRES_PASSWORD and your platform credentials
docker compose up -d --build
```

The site will be available at `https://yourdomain.com` (Caddy handles HTTPS automatically via Let's Encrypt).

### Cloudflare DNS challenge

If your server is behind Cloudflare (proxied DNS), the standard HTTP ACME challenge won't work. Use the Cloudflare compose file instead, which builds a custom Caddy with the Cloudflare DNS plugin:

```bash
docker compose -f docker-compose.cloudflare.yml up -d --build
```

Set `CF_API_TOKEN` in your `.env`. Create the token at https://dash.cloudflare.com/profile/api-tokens with **Zone:DNS:Edit** permission for your domain.

## Local development

Requirements: Go 1.24+, a running PostgreSQL instance.

```bash
DB_URL="postgres://postgres:yourpassword@localhost:5432/bbscope?sslmode=disable" \
  go run *.go serve --dev --poll-interval 0 --listen localhost:7001
```

The `--dev` flag enables HTTP-only mode (no TLS). `--poll-interval 0` disables background polling so you don't need platform credentials.

### Serve command flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dev`, `-d` | `false` | Development mode (HTTP, no TLS) |
| `--poll-interval` | `6` | Hours between polling cycles (0 to disable) |
| `--listen` | `:8080` | HTTP listen address |
| `--domain` | `bbscope.com` | Domain for sitemap/robots.txt |

The database connection is read from `DB_URL` env var or `db_url` in `~/.bbscope.yaml`.

## Configuration

### Platform credentials

All platform credentials are optional. Unconfigured platforms are simply skipped during polling.

| Platform | Env vars | Notes |
|----------|----------|-------|
| HackerOne | `H1_USERNAME`, `H1_TOKEN` | API token |
| Bugcrowd | `BC_EMAIL`, `BC_PASSWORD`, `BC_OTP` | Or set `BC_PUBLIC_ONLY=1` for public programs only |
| Intigriti | `IT_TOKEN` | Bearer token |
| YesWeHack | `YWH_EMAIL`, `YWH_PASSWORD`, `YWH_OTP` | Email + password + OTP |

### AI normalization (optional)

Set `OPENAI_API_KEY` and optionally `OPENAI_MODEL` (defaults to `gpt-4.1-mini`) to enable AI-based scope target normalization. Cached per target to minimize API calls.

### Basic auth (optional)

To protect the site with HTTP basic auth:

1. Generate a password hash:
   ```bash
   docker run --rm caddy:2-alpine caddy hash-password --plaintext 'yourpassword'
   ```

2. Copy the example config into `conf.d/`:
   ```bash
   cp basicauth.caddy.example conf.d/basicauth.caddy
   ```

3. Edit `conf.d/basicauth.caddy` and add your username and hash:
   ```
   basic_auth {
       myuser $2a$14$hashgoeshere...
   }
   ```

4. Restart Caddy: `docker compose restart caddy`

To disable, remove `conf.d/basicauth.caddy` and restart.

## Architecture

```
caddy (ports 80/443) → bbscope-web (:8080) → postgres
```

- **Caddy** handles TLS termination and reverse proxying. Extra config fragments in `conf.d/*.caddy` are auto-imported.
- **bbscope-web** serves the site and runs background pollers.
- **PostgreSQL** stores programs, targets, and scope change history. Schema is auto-migrated on startup.

## API

The site exposes a public API:

| Endpoint | Description |
|----------|-------------|
| `GET /api/v1/programs` | List all programs |
| `GET /api/v1/programs/{platform}/{handle}` | Single program detail |
| `GET /api/v1/targets/{type}` | Targets by type: `wildcards`, `domains`, `urls`, `ips`, `cidrs` |

Query params: `scope` (in/out/both), `platform`, `type`, `raw` (skip AI), `format` (json/text).

Default output is newline-delimited text; add `format=json` for JSON. Responses are cached for 5 minutes.
