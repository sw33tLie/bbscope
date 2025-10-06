<p align="center">
  <img src="logo.svg" alt="bbscope-logo-white" width="500">
</p>

**bbscope** is a powerful scope aggregation tool for bug bounty hunters, designed to fetch, store, and manage program scopes from HackerOne, Bugcrowd, Intigriti, YesWeHack, and Immunefi right from your command line.

Visit [bbscope.com](https://bbscope.com/) to explore an hourly-updated list of public scopes from all supported platforms, stats, and more!

---

## ✨ Features

- **Multi-Platform Support**: Aggregates scopes from all major bug bounty platforms.
- **Local Database**: Stores all scope data in a local SQLite database for fast, offline access.
- **Powerful Querying**: Easily print targets by type (URLs, wildcards, mobile, etc.), platform, or program.
- **Track Changes**: Monitor scope additions and removals over time.
- **Flexible Output**: Get your data in plain text, JSON, or CSV.
- **Direct DB Access**: Drop into an interactive SQL shell to run complex queries directly on the database.

---

## 📦 Installation

Ensure you have a recent version of Go installed, then run:

```bash
go install github.com/sw33tLie/bbscope/v2@refactor/v2
```

**Note:** This command installs directly from the `refactor/v2` branch. Once these changes are merged into the main branch and a new version is tagged, you can switch back to using `@latest`.

---

## 🔐 Configuration

`bbscope` requires API credentials for private programs. After running the tool for the first time, it will create a configuration file at `~/.bbscope.yaml`.

You'll need to fill in your credentials for each platform you want to use:

```yaml
hackerone:
  username: "YOUR_H1_USERNAME"
  token: "YOUR_H1_API_KEY"
bugcrowd:
  email: ""
  password: ""
  otpsecret: "" # Your 2FA secret key
intigriti:
  token: ""
yeswehack:
  email: ""
  password: ""
  otpsecret: "" # Your 2FA secret key
```

Alternatively, you can provide credentials directly via command-line flags when running a `poll` subcommand. Flags will always override values in the configuration file.

**Authentication Flags for `poll` Subcommands:**

| Command | Flag | Description |
| --- | --- | --- |
| `poll h1` | `--user`, `--token` | Your HackerOne username and API token. |
| `poll bc` | `--token` | A live `_bugcrowd_session` cookie. Use as an alternative to email/pass/otp. |
| | `--email`, `--password`, `--otp-secret` | Your Bugcrowd login credentials. |
| `poll it` | `--token` | Your Intigriti authorization token (Bearer). |
| `poll ywh` | `--token` | A live YesWeHack bearer token. Use as an alternative to email/pass/otp. |
| | `--email`, `--password`, `--otp-secret`| Your YesWeHack login credentials. |

---

## 🛠️ Usage & Commands

`bbscope` is organized into two main commands: `poll` and `db`.

### `poll` - Fetching Scopes

The `poll` command fetches scope data from the platforms. You can poll all platforms at once or specify which ones to poll.

**Subcommands:**

- `bbscope poll`: Polls all configured platforms.
- `bbscope poll h1`: Polls HackerOne.
- `bbscope poll bc`: Polls Bugcrowd.
- `bbscope poll it`: Polls Intigriti.
- `bbscope poll ywh`: Polls YesWeHack.
- `bbscope poll immunefi`: Polls Immunefi (no authentication required).

**Flags for `poll`:**

| Flag | Description | Default |
| --- | --- | --- |
| `--db` | Save results to the database and print changes. | `false` |
| `-b, --bbpOnly` | Only fetch programs offering monetary rewards. | `false` |
| `-p, --pvtOnly` | Only fetch data from private programs. | `false` |
| `--category` | Scope categories to include (e.g., `url`, `cidr`, `mobile`). | `"all"` |
| `-o, --output` | Output flags for printing to stdout (`t`=target, `d`=description, `c`=category, `u`=program URL). | `"tu"` |
| `-d, --delimiter` | Delimiter for `txt` output when using multiple output flags. | `" "` |
| `--oos` | Include out-of-scope targets in the output. | `false` |
| `--concurrency`| Number of concurrent fetches per platform. | `5` |
| `--dbpath` | Path to the SQLite database file. | `"bbscope.sqlite"`|

---

### `db` - Interacting with the Database

The `db` command lets you query and manage the data stored in your local SQLite database.

#### `db print`

Prints scope data from the database.

**Usage:** `bbscope db print [type]`

-   **`type`** (optional): Filter by target type. Can be `urls`, `wildcards`, `apis`, or `mobile`. If omitted, prints all types.

**Flags for `db print`:**

| Flag | Description | Default |
| --- | --- | --- |
| `--platform` | Comma-separated platforms to filter by (e.g., `h1,bc`), or `all`. | `"all"` |
| `--program` | Filter by a specific program handle or URL. | |
| `--format` | Output format: `txt`, `json`, or `csv`. | `"txt"` |
| `-o, --output` | Output flags for `txt` format (`t`=target, `d`=description, `c`=category, `u`=program URL). | `"tu"` |
| `-d, --delimiter` | Delimiter for `txt` output. | `" "` |
| `--oos` | Include out-of-scope targets. | `false` |
| `--since` | Show targets added since a given RFC3339 timestamp (e.g., `2023-10-27T10:00:00Z`). | |

#### `db stats`

Shows high-level statistics about the data in the database.

**Usage:** `bbscope db stats`

#### `db changes`

Shows the most recent scope changes (additions/removals).

**Usage:** `bbscope db changes`

| Flag | Description | Default |
| --- | --- | --- |
| `--limit` | Number of recent changes to show. | `50` |

#### `db shell`

Opens an interactive `sqlite3` shell to the database, allowing you to run any SQL query you want. It prints the database schema upon opening.

**Usage:** `bbscope db shell`

**Persistent Flag for `db`:**

All `db` subcommands accept the `--dbpath` flag to specify the location of the database file.

---

## 📖 Examples

**1. First-Time Setup: Poll all private, bounty-only programs and save to DB**

This is a great first command to run to populate your database.

```bash
bbscope poll --db -b -p
```

**2. Print all wildcard targets from HackerOne and Bugcrowd**

```bash
bbscope db print wildcards --platform h1,bc
```

**3. Get all targets for a specific program in JSON format**

```bash
bbscope db print --program "hackerone" --format json
```

**4. Show the 10 most recent scope changes**

```bash
bbscope db changes --limit 10
```

**5. Get a unique list of program URLs from Intigriti that have bounties**

```bash
bbscope db print --platform it --output u | sort -u
```
