# Hermes MCP configuration

The NAS MCP service is deployed as:

- binary `0.2.0`, commit `c7f996b30d9e055ad71511307bbdd77184c667c9`
- service `release-watcher-mcp.service`
- bind `127.0.0.1:8741` (not public)
- endpoint `/mcp`
- write mode enabled on the server
- token file `/etc/release-watcher/mcp.env` (root:root, `0600`)

## 1. Retrieve the token

Run this on the NAS, not in a chat or a Git-tracked file:

```sh
sudo cat /etc/release-watcher/mcp.env
```

Copy the `RELEASE_WATCHER_MCP_TOKEN` value into Hermes' protected
`~/.hermes/.env`. Do not paste the token into `config.yaml`, shell history,
Git, logs or messages.

## 2. Provide a secure route

The MCP process is loopback-only. Hermes must not connect over plain HTTP from
another machine.

Preferred: use an existing private VPN or an existing HTTPS reverse proxy.
The proxy must preserve the public Host and forward to
`http://127.0.0.1:8741/mcp`. Add the public hostname to the NAS config and
restart the service:

```sh
# edit /etc/release-watcher/mcp.env
RELEASE_WATCHER_MCP_ALLOWED_HOSTS=mcp.private.example
sudo systemctl restart release-watcher-mcp.service
```

See `nginx-mcp.conf.example` for a minimal TLS proxy example. Do not use
`ssl_verify: false` in Hermes.

If there is no private HTTPS/VPN route, remote Hermes deployment remains
blocked. An SSH local-forward (`ssh -L 8741:127.0.0.1:8741 ...`) can be used only
for a local validation session; it is not the production transport and gives
Hermes SSH credentials, which this integration intentionally avoids.

## 3. Add the MCP server to Hermes

Merge `config.example.yaml` into `~/.hermes/config.yaml` without replacing the
existing `mcp_servers` entries. Set `RELEASE_WATCHER_MCP_TOKEN` in
`~/.hermes/.env`, then restart Hermes and verify:

```sh
hermes mcp list
hermes mcp test release_watcher
```

The agent should discover `list_watches`, `get_watch`, `list_events`,
`add_watch`, `pause_watch`, `resume_watch`, `set_watch_interval` and
`remove_watch`. Mutations require explicit user confirmation in Hermes; the
server also enforces write mode and never exposes purge, backup, restore,
shell, filesystem or systemd tools.

## 4. Token rotation

On the NAS:

```sh
sudo scripts/install-mcp-env.sh --force --write-enabled
```

Then update `RELEASE_WATCHER_MCP_TOKEN` in Hermes' `.env` and restart both the
MCP service and Hermes. The old token stops working immediately after the MCP
restart.
