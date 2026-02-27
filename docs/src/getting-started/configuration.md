# Configuration

bbscope reads configuration from `~/.bbscope.yaml` by default. Override with `--config path/to/file.yaml`.

## Config file

```yaml
hackerone:
  username: "your_h1_username"
  token: "your_h1_api_token"

bugcrowd:
  email: "you@example.com"
  password: "your_password"
  otpsecret: "YOUR_TOTP_SECRET"

intigriti:
  token: "your_intigriti_token"

yeswehack:
  email: "you@example.com"
  password: "your_password"
  otpsecret: "YOUR_TOTP_SECRET"

db:
  url: "postgres://user:pass@localhost:5432/bbscope?sslmode=disable"

ai:
  api_key: "sk-..."
  model: "gpt-4.1-mini"
  # provider: "openai"      # default
  # endpoint: ""             # custom OpenAI-compatible endpoint
  # max_batch: 0             # items per API call (0 = auto)
  # max_concurrency: 0       # parallel API calls (0 = auto)
  # proxy: ""                # HTTP proxy for AI calls
```

## Global CLI flags

These flags apply to all commands:

| Flag | Description |
|------|-------------|
| `--config` | Config file path (default `~/.bbscope.yaml`) |
| `--proxy` | HTTP proxy URL for platform requests |
| `--loglevel` | Log level: `debug`, `info`, `warn`, `error`, `fatal` |
| `--debug-http` | Log full HTTP requests and responses |

## Environment variables

For the web server (`bbscope serve`), credentials are read from environment variables instead of the config file. See [Self-Hosting](../web/self-hosting.md) for the full list.

The `OPENAI_API_KEY` environment variable is used as a fallback when `ai.api_key` is not set in the config file.
