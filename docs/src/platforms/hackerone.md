# HackerOne

## Authentication

HackerOne uses API token authentication (HTTP Basic Auth).

Generate a token at [hackerone.com/settings/api_token](https://hackerone.com/settings/api_token).

### Config file

```yaml
hackerone:
  username: "your_username"
  token: "your_api_token"
```

### CLI flags

```bash
bbscope poll h1 --user your_username --token your_api_token
```

### Environment variables (web server)

```
H1_USERNAME=your_username
H1_TOKEN=your_api_token
```

## What it fetches

- All programs you have access to (public + private if invited)
- In-scope and out-of-scope targets with categories and descriptions
- Paginated via the HackerOne API v1 (`/v1/hackers/programs`)

## Filtering

```bash
# Only bug bounty programs
bbscope poll h1 -b

# Only private programs
bbscope poll h1 -p
```

## Platform name

Used in database records and API responses: **`h1`**

## Program metadata captured

The HackerOne poller extracts limited program-level metadata (queryable via
`bbscope db program h1/<handle>`):

- Title, tagline, program type (bug-bounty/vdp)
- Bounty/VDP flags, public/private status, disabled flag

HackerOne's public hacker API does not expose reward grids, qualifying
vulnerability lists, or testing instructions. These fields are left empty.
