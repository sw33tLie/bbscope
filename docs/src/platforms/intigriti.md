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
