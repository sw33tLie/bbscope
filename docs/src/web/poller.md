# Background Poller

When running `bbscope serve`, a background poller automatically polls all configured platforms on a schedule.

## How it works

1. On startup, the poller runs immediately.
2. After each cycle, it waits for the configured interval (default: 6 hours).
3. All platforms are polled concurrently.
4. After each cycle, the web cache is invalidated so the UI reflects fresh data.

## Configuration

Set the interval via flag or environment variable:

```bash
# Via flag
bbscope serve --poll-interval 12

# Via environment variable (Docker)
POLL_INTERVAL=12
```

## Monitoring

The `/debug` endpoint shows the status of each platform's poller:

- **Last run**: When the poller last ran for this platform
- **Duration**: How long the poll took
- **Status**: Success, failure, or skipped (no credentials configured)

## Behavior differences from CLI

| Aspect | CLI (`bbscope poll --db`) | Web (background poller) |
|--------|--------------------------|------------------------|
| Platform order | Sequential | Concurrent (all at once) |
| Change output | Printed to stdout | Logged to server logs |
| Credentials source | Config file | Environment variables |
| Authentication | Once at startup | Re-authenticated each cycle |
| Error handling | Returns first error | Logs errors, continues |
