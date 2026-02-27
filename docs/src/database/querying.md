# Querying & Searching

## CLI queries

```bash
# Full-text search
bbscope db find "example.com"

# Dump all in-scope targets
bbscope db print

# Filter by platform
bbscope db print --platform h1

# Filter by program
bbscope db print --program "https://hackerone.com/example"

# Changes since a date
bbscope db print --since 2025-01-01T00:00:00Z

# JSON output for scripting
bbscope db print --format json
```

## Target extraction

The `db get` subcommands extract cleaned, deduplicated targets ready for recon:

```bash
bbscope db get wildcards          # *.example.com
bbscope db get wildcards --aggressive  # root domains via publicsuffix
bbscope db get domains            # app.example.com
bbscope db get urls               # https://app.example.com/api
bbscope db get ips                # 1.2.3.4
bbscope db get cidrs              # 10.0.0.0/8
```

See [Extracting Targets](../cli/targets.md) for details.

## Direct SQL

For advanced queries, use the built-in shell:

```bash
bbscope db shell
```

This opens `psql` with the schema printed for reference. Example queries:

```sql
-- Programs with most in-scope targets
SELECT p.url, COUNT(*) as target_count
FROM programs p
JOIN targets_raw t ON t.program_id = p.id
WHERE t.in_scope = 1
GROUP BY p.url
ORDER BY target_count DESC
LIMIT 20;

-- Recent wildcard additions
SELECT sc.occurred_at, sc.program_url, sc.target_raw
FROM scope_changes sc
WHERE sc.change_type = 'added'
  AND sc.category = 'wildcard'
ORDER BY sc.occurred_at DESC
LIMIT 50;

-- Programs added in the last 7 days
SELECT url, platform, first_seen_at
FROM programs
WHERE first_seen_at > NOW() - INTERVAL '7 days'
ORDER BY first_seen_at DESC;
```

## REST API

When running the web server (`bbscope serve`), the same data is available via the REST API. See [REST API](../web/api.md).
