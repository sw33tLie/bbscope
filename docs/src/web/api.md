# REST API

The web server exposes a REST API under `/api/v1/`. CORS is enabled for all origins.

## Programs

### List all programs

```
GET /api/v1/programs
```

Returns all programs with their scope data. Response is cached for 5 minutes.

Query parameters:

| Param | Description |
|-------|-------------|
| `raw` | Set to `true` to return raw (non-AI-normalized) scope data |

### Get a single program

```
GET /api/v1/programs/{platform}/{handle}
```

Returns full scope data for a specific program.

## Targets

Extract specific target types across all programs.

```
GET /api/v1/targets/wildcards
GET /api/v1/targets/domains
GET /api/v1/targets/urls
GET /api/v1/targets/ips
GET /api/v1/targets/cidrs
```

Query parameters:

| Param | Description |
|-------|-------------|
| `scope` | `in` (default), `out`, or `all` |
| `platform` | Filter by platform name |
| `type` | Filter by target category |
| `raw` | `true` for raw (non-AI) data |
| `format` | `text` (default) or `json` |

### Examples

```bash
# All in-scope wildcards as plain text
curl https://your-instance.com/api/v1/targets/wildcards

# HackerOne wildcards as JSON
curl "https://your-instance.com/api/v1/targets/wildcards?platform=h1&format=json"

# Out-of-scope URLs
curl "https://your-instance.com/api/v1/targets/urls?scope=out"
```

## Updates

```
GET /api/v1/updates
```

Returns paginated scope changes.

Query parameters:

| Param | Description |
|-------|-------------|
| `since` | Time filter: `today`, `yesterday`, `7d`, `30d`, `90d`, `1y`, or an ISO date (`YYYY-MM-DD`) |
| `page` | Page number (default: 1) |

### Example

```bash
# Changes in the last 7 days
curl "https://your-instance.com/api/v1/updates?since=7d"
```
