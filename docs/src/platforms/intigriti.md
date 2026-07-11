# Intigriti

## Authentication

Intigriti uses a bearer token.

### Config file

```yaml
intigriti:
  token: "your_intigriti_token"
```

### CLI flags

```bash
bbscope poll it --token your_token
```

### Environment variables (web server)

```
IT_TOKEN=your_token
```

## What it fetches

- All accessible programs via the Intigriti researcher API
- Two-pass scope processing to accurately detect BBP status from tier values
- In-scope and out-of-scope targets with categories

## Platform name

Used in database records and API responses: **`it`**

## Program metadata captured

The Intigriti poller extracts and stores the following program-level metadata
(queryable via `bbscope db program it/<company>/<handle>`):

- Title, industry, program type (bug-bounty/vdp), public/private, disabled flags
- Currency, bounty reward min/max (from listing endpoint)
- Rules (markdown), rules format
- Testing requirements: User-Agent, required request header, automated tooling limit
- Safe harbor status
- Account creation capability (intigritiMe flag)
- Scope count
