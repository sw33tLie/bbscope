# Database Setup

bbscope uses PostgreSQL to store program scopes and track changes over time.

## Requirements

- PostgreSQL 14+ (16 recommended)

## Creating the database

```bash
createdb bbscope
```

Or via `psql`:

```sql
CREATE DATABASE bbscope;
```

## Connection string

Add to `~/.bbscope.yaml`:

```yaml
db:
  url: "postgres://user:password@localhost:5432/bbscope?sslmode=disable"
```

Or pass via flag:

```bash
bbscope db stats --db-url "postgres://user:password@localhost:5432/bbscope?sslmode=disable"
```

Or via environment variable (used by the web server):

```bash
export DB_URL="postgres://user:password@localhost:5432/bbscope?sslmode=disable"
```

## Schema auto-migration

bbscope automatically creates all tables and indexes on first connection. There's no manual migration step. The schema is idempotent — safe to run against an existing database.

## Docker

For local development, a quick PostgreSQL instance:

```bash
docker run -d \
  --name bbscope-db \
  -e POSTGRES_DB=bbscope \
  -e POSTGRES_USER=bbscope \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16-alpine
```

Then:

```yaml
db:
  url: "postgres://bbscope:secret@localhost:5432/bbscope?sslmode=disable"
```
