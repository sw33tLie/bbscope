# AI Normalization — Configuration

## Config file

```yaml
ai:
  api_key: "sk-..."
  model: "gpt-4.1-mini"       # default
  # provider: "openai"         # default
  # endpoint: ""               # custom OpenAI-compatible endpoint
  # max_batch: 0               # items per API call (0 = auto)
  # max_concurrency: 0         # parallel API calls (0 = auto)
  # proxy: ""                  # HTTP proxy for AI API calls
```

## Environment variable fallback

If `ai.api_key` is not set in the config, bbscope falls back to the `OPENAI_API_KEY` environment variable.

For the web server, use:

```
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4.1-mini
```

## Usage

### CLI

```bash
bbscope poll --db --ai
```

The `--ai` flag only works with `--db`. Without a database, there's nowhere to store the normalized results.

### Web server

If `OPENAI_API_KEY` is set, AI normalization is automatically enabled for the background poller. Check the `/debug` page to confirm.

## Custom endpoints

For self-hosted or alternative OpenAI-compatible APIs (e.g., Ollama, vLLM, Azure OpenAI), set the endpoint:

```yaml
ai:
  api_key: "your-key"
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
```

## Cost considerations

- Only **new or changed** targets are sent to the API. Previously normalized targets are loaded from the database cache.
- The default model (`gpt-4.1-mini`) is cost-effective for this use case.
- Batch size and concurrency can be tuned with `max_batch` and `max_concurrency` if needed.
