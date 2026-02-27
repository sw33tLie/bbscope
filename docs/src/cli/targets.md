# Extracting Targets

The `bbscope db get` commands extract specific target types from the database, formatted for direct use with recon tools.

## Wildcards

```bash
# Standard wildcard extraction
bbscope db get wildcards

# Aggressive mode: extracts root domains even from subdomains
bbscope db get wildcards --aggressive

# Filter by platform
bbscope db get wildcards --platform h1
```

Wildcards are deduplicated and sorted. Shared hosting / cloud provider domains are automatically filtered out (e.g., `*.amazonaws.com`, `*.herokuapp.com`, `*.azurewebsites.net`, and ~20 others).

### Piping to recon tools

```bash
# Subdomain enumeration
bbscope db get wildcards | subfinder -silent | httpx -silent

# Aggressive mode for broader coverage
bbscope db get wildcards --aggressive | subfinder -silent
```

## Domains

```bash
bbscope db get domains
```

Returns non-URL, non-IP targets that contain a dot (e.g., `app.example.com`).

## URLs

```bash
bbscope db get urls
```

Returns targets starting with `http://` or `https://`.

## IPs

```bash
bbscope db get ips
```

Extracts IP addresses, including from URL targets.

## CIDRs

```bash
bbscope db get cidrs
```

Returns CIDR ranges and IP ranges from scope entries.

## Common flags

All `db get` subcommands support:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--platform` | | | Filter by platform name |
| `--output` | `-o` | `t` | Output flags |
| `--delimiter` | `-d` | `" "` | Delimiter |
