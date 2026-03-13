# AI Normalization — Overview

Bug bounty platforms often have messy scope entries. Targets might look like:

- `*.example.com (main website)` instead of `*.example.com`
- `https://app.example.com/api/v2 - REST API` instead of `https://app.example.com/api/v2`
- `All subdomains of example.com` instead of `*.example.com`

AI normalization uses an LLM (OpenAI-compatible API) to clean these up automatically.

## What it does

1. **Cleans up entries** — strips descriptions, comments, and formatting artifacts from target strings.
2. **Handles wildcards** — converts "All subdomains of X" to `*.X`.
3. **Classifies scope intent** — determines if a messy entry is a wildcard, URL, domain, etc.
4. **Normalizes categories** — maps platform-specific category names to unified ones.
5. **Caches results** — stores AI outputs in the `targets_ai_enhanced` database table. Only new/changed targets are sent to the API on subsequent polls, avoiding redundant API calls and costs.

## How it looks

In the web UI, the program detail page has an "AI / Raw" toggle to switch between views.

In the CLI with `--db`, changes show the mapping:

```
🆕  h1  https://hackerone.com/program  *.example.com (main site) -> *.example.com
```

## Requirements

- A database (`--db` flag)
- An OpenAI-compatible API key

See [Configuration](./configuration.md) for setup.
