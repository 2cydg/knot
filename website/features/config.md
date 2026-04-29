# Servers and Global Config

Knot configuration contains servers, proxies, managed keys, and global settings. Sensitive values are stored with the `ENC:` prefix and encrypted through platform facilities or a machine-bound Linux fallback.

## Server Commands

| Command | Description |
| --- | --- |
| `knot add [alias]` | Add or overwrite a server configuration. |
| `knot edit [alias]` | Edit an existing server configuration. |
| `knot list [pattern]` | List servers. Alias: `knot ls`. |
| `knot remove [alias]` | Remove a server. Aliases: `knot rm`, `knot delete`. |

Common server fields:

| Field | Source Flag | Description |
| --- | --- | --- |
| Alias | `[alias]` or `--alias` | Server alias used by most commands. |
| Host | `--host` | Host name or IP address. |
| Port | `--port` | SSH port, default `22`. |
| User | `--user` | SSH username. |
| Auth | `--auth-method`, `--password`, `--key` | Password, managed private key, or agent authentication. |
| Jump hosts | `--jump-host` | Comma-separated jump host alias chain. |
| Proxy | `--proxy` | Managed proxy alias. |
| Tags | `--tags` | Comma-separated tags for filtering and organization. |

## Global Config

```sh
knot config [command]
```

| Command | Description |
| --- | --- |
| `knot config init` | Initialize config or reset global settings to defaults. Existing servers, proxies, and keys are preserved. |
| `knot config list` | List global settings. Alias: `knot config ls`. |
| `knot config get [path]` | Print the full sanitized config or one sanitized path. |
| `knot config set [key] [value]` | Set a global setting. |

In `knot config get`, paths without dots are resolved under `settings` first:

```sh
knot config get
knot config get log_level
knot config get servers.web-prod
```

## Settable Keys

| Key | Value Type | Description |
| --- | --- | --- |
| `forward_agent` | bool | Whether to forward the SSH agent. |
| `clear_screen_on_connect` | bool | Whether to clear the screen after connecting. |
| `idle_timeout` | Go duration | Idle timeout for daemon-held connections. |
| `keepalive_interval` | Go duration | SSH keepalive interval. |
| `log_level` | `debug`, `info`, `warn`, `error` | Log level. |
| `default_sync_provider` | provider alias | Default provider used by `knot sync push` and `knot sync pull`. |

Examples:

```sh
knot config set forward_agent true
knot config set idle_timeout 30m
knot config set log_level error
knot config set default_sync_provider home
```

Config changes apply to new connections.

## Import and Export

```sh
knot export [path]
knot import [path]
```

`knot export` writes a password-encrypted archive. The default output path is `config.toml.enc`.

`knot import` reads a password-encrypted archive and lets you choose a merge strategy:

```sh
knot export backup.enc
knot import backup.enc
```

For day-to-day multi-device sharing, use [Config Sync](/features/sync). Sync only includes `servers`, `proxies`, and `keys`, and uses WebDAV as the remote provider.
