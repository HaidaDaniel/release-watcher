# Hermes MCP configuration

The NAS MCP service is deployed as:

- binary `0.2.0`, commit `c7f996b30d9e055ad71511307bbdd77184c667c9`
- service `release-watcher-mcp.service`
- app bind `127.0.0.1:8741` (not public)
- LAN HTTPS reverse proxy `https://<NAS_LAN_IP>:8742/mcp`, bound to the LAN
  interface only and restricted to the LAN CIDR
- self-signed CA `/etc/release-watcher/tls/mcp-ca.crt` on the NAS
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

## 2. Trust the LAN CA on the Hermes PC

Copy the public CA certificate (not the private key) from the NAS:

```sh
scp <nas-user>@<NAS_LAN_IP>:/etc/release-watcher/tls/mcp-ca.crt .
```

On a Debian/Ubuntu Hermes host:

```sh
sudo cp mcp-ca.crt /usr/local/share/ca-certificates/release-watcher-mcp.crt
sudo update-ca-certificates
```

If Hermes uses its own Python trust store, set `SSL_CERT_FILE` to the copied
`mcp-ca.crt` for the Hermes process. Never use `ssl_verify: false`.

The LAN endpoint is `https://<NAS_LAN_IP>:8742/mcp`. It is bound to the NAS LAN
interface and restricted to the LAN CIDR; port 8741 itself remains loopback and
there is no public internet exposure.

If the NAS LAN IP or hostname changes, update the certificate SAN, the nginx
`server_name`/`listen` and `RELEASE_WATCHER_MCP_ALLOWED_HOSTS`, then restart the
MCP service and nginx. For access from outside the LAN, use an existing private
VPN/HTTPS route instead of port-forwarding this port to the internet.

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
sudo scripts/install-mcp-env.sh --force --write-enabled \
  --allowed-hosts <NAS_LAN_IP>,<NAS_HOSTNAME>,<NAS_HOSTNAME>.local
```

Then update `RELEASE_WATCHER_MCP_TOKEN` in Hermes' `.env` and restart both the
MCP service and Hermes. The old token stops working immediately after the MCP
restart. If the script is not on the NAS, copy it from the deployment checkout
or run the equivalent `openssl rand -hex 32` command directly on the NAS.
