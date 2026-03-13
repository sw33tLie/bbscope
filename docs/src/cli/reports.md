# Downloading Reports

The `bbscope reports` command downloads your vulnerability reports from bug bounty platforms as Markdown files.

## HackerOne

```bash
# Download all your reports
bbscope reports h1 --output-dir ./reports

# Preview what would be downloaded (dry-run)
bbscope reports h1 --output-dir ./reports --dry-run

# Filter by program
bbscope reports h1 --output-dir ./reports --program google --program microsoft

# Filter by state (e.g., resolved, triaged, new, duplicate, informative, not-applicable, spam)
bbscope reports h1 --output-dir ./reports --state resolved --state triaged

# Filter by severity
bbscope reports h1 --output-dir ./reports --severity critical --severity high

# Combine filters
bbscope reports h1 --output-dir ./reports --program google --state resolved --severity critical

# Overwrite existing files
bbscope reports h1 --output-dir ./reports --overwrite
```

### Authentication

Credentials can be provided via CLI flags or config file:

```bash
# CLI flags
bbscope reports h1 --user your_username --token your_api_token --output-dir ./reports
```

```yaml
# ~/.bbscope.yaml
hackerone:
  username: "your_username"
  token: "your_api_token"
```

### Output structure

Reports are saved as Markdown files organized by program:

```
reports/
└── h1/
    ├── google/
    │   ├── 123456_XSS_in_login_page.md
    │   └── 123457_IDOR_in_user_profile.md
    └── microsoft/
        └── 234567_SSRF_in_webhook_handler.md
```

Each file contains a metadata table (ID, program, state, severity, weakness, asset, bounty, CVE IDs, timestamps) followed by the vulnerability information and impact sections.

### Dry-run output

The `--dry-run` flag prints a table of matching reports without downloading:

```
ID       PROGRAM    STATE     SEVERITY  CREATED                    TITLE
123456   google     resolved  high      2024-01-15T10:30:00.000Z   XSS in login page
123457   google     triaged   critical  2024-02-20T14:00:00.000Z   IDOR in user profile
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--output-dir` | | Output directory for downloaded reports (required) |
| `--program` | | Filter by program handle(s) |
| `--state` | | Filter by report state(s) |
| `--severity` | | Filter by severity level(s) |
| `--dry-run` | | List reports without downloading |
| `--overwrite` | | Overwrite existing report files |

## How it works

1. **List phase**: fetches all report summaries from the HackerOne API (`/v1/hackers/me/reports`), paginated at 100 per page. Filters are applied server-side using Lucene query syntax.
2. **Download phase**: 10 parallel workers fetch full report details (`/v1/hackers/reports/{id}`) and write them as Markdown files.
3. **Skip logic**: existing files are skipped unless `--overwrite` is set.
4. **Rate limiting**: HTTP 429 responses trigger a 60-second backoff. Other transient errors are retried up to 3 times with a 2-second delay.

> **Note**: The HackerOne Hacker API may not return draft reports or reports where you are a collaborator but not the primary reporter. If your downloaded count is lower than your dashboard total, this is likely the cause.
