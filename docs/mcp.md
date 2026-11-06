# MCP integration

Release Watcher exposes the existing `internal/app` use cases over the Model
Context Protocol Streamable HTTP transport. It is not a second API or database:
every tool calls `app.Service`.

## Endpoint

- Command: `release-watcher mcp serve`
- Default listen: `127.0.0.1:8741`
- Path: `/mcp`
- Transport: Streamable HTTP (no legacy SSE-only endpoint)
- SDK: `github.com/modelcontextprotocol/go-sdk v1.8.0`
- Protocol: SDK v1.8.0 supports MCP `2026-07-28` and older revisions (`2025-11-25`,
  `2025-06-18`, `2025-03-26`, `2024-11-05`). Hermes' current Python MCP client
  implements `2026-07-28`; the SDK negotiates a mutually supported revision.
- Stateless mode: no persistent MCP session table; GET/DELETE session endpoints
  are not used.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `RELEASE_WATCHER_MCP_TOKEN` | unset | required Bearer token; at least 32 bytes, generate with `openssl rand -hex 32` |
| `RELEASE_WATCHER_MCP_LISTEN` | `127.0.0.1:8741` | HTTP bind address |
| `RELEASE_WATCHER_MCP_WRITE_ENABLED` | `false` | expose and authorize mutation tools |
| `RELEASE_WATCHER_MCP_ALLOWED_HOSTS` | loopback | comma-separated extra Host values |
| `RELEASE_WATCHER_MCP_ALLOWED_ORIGINS` | empty | comma-separated allowed browser Origin values |

The token is accepted only in `Authorization: Bearer ...`; it is never accepted
in a URL/query string and is never logged or returned. Missing, empty, malformed
and wrong tokens get `401` without information about watches.

## Tools

Read-only (available in both modes):

- `list_watches` — active watches, capped at 200 with a `truncated` flag.
- `get_watch` — one watch with bounded releases, events and scans.
- `list_events` — events with `limit` (1–100), `after` cursor, `since`,
  `publication_id` and `watch_id` filters.

Mutations (only when `RELEASE_WATCHER_MCP_WRITE_ENABLED=true`):

- `add_watch` — URL, optional interval and optional Russian title; uses the
  existing SSRF validation and silent baseline.
- `pause_watch`, `resume_watch` — existing scheduling semantics.
- `set_watch_interval` — duration such as `24h` or `48h`.
- `remove_watch` — stops watching, preserves history and cancels pending
  notifications. Never calls `purge`.

MCP mutation throttling returns the stable code `mcp_rate_limited`; it is
separate from the source `rate_limited` error.

Not exposed: `purge`, `backup`, `restore`, raw SQL, shell, filesystem, systemd,
token changes, arbitrary scans and arbitrary Telegram messages.

Mutations should be confirmed by the user in Hermes; the server also enforces
the write-mode switch, which is the actual authorization boundary.

## Example Hermes entry

```yaml
mcp_servers:
  release_watcher:
    url: "https://<PRIVATE_MCP_HOST>/mcp"
    headers:
      Authorization: "Bearer ${RELEASE_WATCHER_MCP_TOKEN}"
    ssl_verify: true
    enabled: true
    timeout: 45
    connect_timeout: 10
    supports_parallel_tool_calls: false
    tools:
      include:
        - list_watches
        - get_watch
        - list_events
        - add_watch
        - pause_watch
        - resume_watch
        - set_watch_interval
        - remove_watch
```

Store the token in Hermes' protected `.env`/secret scope, not in `config.yaml`.
Do not use `ssl_verify: false`. Remote deployment requires a private HTTPS/VPN
route; the default server binds loopback only.
