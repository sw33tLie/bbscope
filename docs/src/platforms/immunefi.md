# Immunefi

## Authentication

Immunefi requires **no authentication**. All data is scraped from the public Immunefi website.

## Usage

```bash
bbscope poll immunefi
```

No configuration needed. Works out of the box.

## Notes

- Fetches data from the public Immunefi bug bounty listings.
- Uses exponential backoff for rate limiting (HTTP 429 responses).
- Parses React Server Components (RSC) responses from the Immunefi website.

## Platform name

Used in database records and API responses: **`immunefi`**
