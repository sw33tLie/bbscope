# Targets Package

```go
import "github.com/sw33tLie/bbscope/v2/pkg/targets"
```

The `targets` package extracts specific target types from scope entries. It handles deduplication, sorting, and filtering.

## Functions

All functions take `[]storage.Entry` and return `[]string`:

```go
// Wildcards — with automatic filtering of shared hosting domains
wildcards := targets.CollectWildcardsSorted(entries, aggressive)

// Domains — non-URL, non-IP targets containing a dot
domains := targets.CollectDomains(entries)

// URLs — targets starting with http:// or https://
urls := targets.CollectURLs(entries)

// IPs — IP addresses, including extracted from URLs
ips := targets.CollectIPs(entries)

// CIDRs — CIDR ranges and IP ranges
cidrs := targets.CollectCIDRs(entries)
```

### Out-of-scope variants

Each function has an OOS counterpart:

```go
oosWildcards := targets.CollectOOSWildcards(entries)
oosDomains   := targets.CollectOOSDomains(entries)
oosURLs      := targets.CollectOOSURLs(entries)
oosIPs       := targets.CollectOOSIPs(entries)
oosCIDRs     := targets.CollectOOSCIDRs(entries)
```

## Wildcard filtering

`CollectWildcardsSorted` automatically filters out wildcards for shared hosting and cloud provider domains that would generate too many false positives:

- `*.amazonaws.com`, `*.cloudfront.net`
- `*.azurewebsites.net`, `*.azure.com`
- `*.herokuapp.com`, `*.netlify.app`
- `*.shopify.com`, `*.myshopify.com`
- And ~15 more

### Aggressive mode

When `aggressive` is `true`, root domains are extracted via the public suffix list. For example, `app.staging.example.com` becomes `*.example.com`.

## Subdomain tool normalization

```go
// Clean a scope string for use with subdomain enumeration tools
cleaned := targets.NormalizeForSubdomainTools(scopeEntry)
```

Strips prefixes like `*.`, `http://`, trailing paths, and other artifacts.

## Example

```go
db, _ := storage.Open(dbURL)
defer db.Close()

entries, _ := db.ListEntries(ctx, storage.ListOptions{InScope: true})

wildcards := targets.CollectWildcardsSorted(entries, false)
for _, w := range wildcards {
    fmt.Println(w)
}
```
