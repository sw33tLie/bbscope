# Docker Deployment

The recommended way to deploy the web interface is with Docker Compose. The setup includes PostgreSQL, the bbscope web server, and Caddy as a reverse proxy with automatic HTTPS.

## Quick start

```bash
cd website/
cp .env.example .env
# Edit .env with your credentials
docker compose up -d
```

## Architecture

```
Internet → Caddy (443/80) → bbscope-web (8080) → PostgreSQL (5432)
```

Three services:

1. **postgres** (PostgreSQL 16 Alpine) — data storage with health checks and a persistent volume
2. **bbscope-web** — the bbscope web server, polls platforms on a schedule
3. **caddy** (Caddy 2 Alpine) — reverse proxy with automatic TLS certificate management

## Environment variables

Create a `.env` file from the template:

```bash
cp .env.example .env
```

Edit with your values. At minimum, set:

- `POSTGRES_PASSWORD` — database password
- `DOMAIN` — your public domain (used by Caddy for HTTPS)
- At least one platform's credentials

## Cloudflare DNS challenge

For wildcard certificates or when port 80 isn't available for HTTP challenges, use the Cloudflare DNS challenge variant:

```bash
docker compose -f docker-compose.cloudflare.yml up -d
```

This builds a custom Caddy image with the Cloudflare DNS plugin. Set `CF_API_TOKEN` in your `.env` file.

## Updating

```bash
docker compose pull
docker compose up -d
```

Or rebuild from source:

```bash
docker compose up -d --build
```

## Logs

```bash
# All services
docker compose logs -f

# Just the web server
docker compose logs -f bbscope-web

# Just the poller output
docker compose logs -f bbscope-web | grep "Poller:"
```

## Data persistence

PostgreSQL data is stored in a named volume (`bbscope-pgdata`). To back up:

```bash
docker compose exec postgres pg_dump -U bbscope bbscope > backup.sql
```
