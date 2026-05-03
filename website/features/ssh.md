# SSH Connections

SSH is Knot's core entry point. Save a server alias with `knot add`, then open an interactive SSH session with `knot ssh [alias]`. The background daemon keeps reusable connections alive so later `exec`, `sftp`, `cp`, and forwarding operations can reuse the same connection path.

## `knot ssh`

```sh
knot ssh [alias]
```

| Argument | Required | Description |
| --- | --- | --- |
| `[alias]` | Yes | Server alias to connect to. |

Example:

```sh
knot ssh web-prod
```

### Shortcut

When the root command receives an unknown first argument, it treats it as a server alias and rewrites the invocation to `knot ssh [alias]`:

```sh
knot web-prod
```

Equivalent to:

```sh
knot ssh web-prod
```

To avoid ambiguity, shortcut aliases cannot match built-in commands and cannot contain whitespace, path separators, or common shell metacharacters.

### Command Broadcast

Interactive SSH sessions can join a daemon-local broadcast group. Input typed in one active member is forwarded to the other active members in the same group:

```sh
knot ssh web-1 --broadcast cloud
knot web-2 --broadcast cloud
```

Use `knot broadcast list` and `knot broadcast show cloud` to inspect groups. `knot broadcast pause`, `resume`, `leave`, and `disband` manage existing sessions without closing the SSH connection.

Session-local escape controls are disabled by default. Enable them for one connection with `--escape`, set a custom one-character prefix with `--escape %`, or force them off with `--escape none`:

```sh
knot ssh web-1 --broadcast cloud --escape
knot ssh web-1 --broadcast cloud --escape %
knot ssh web-1 --broadcast cloud --escape none
```

The global settings `broadcast_escape_enable` and `broadcast_escape_char` control the default for SSH sessions started with `--broadcast`. Command-line `--escape` always has priority over the global settings. Plain `knot ssh [alias]` sessions do not enable broadcast escapes from the global settings. When escape controls are enabled, Knot prints the available local shortcuts after connecting.

## Add a Server

```sh
knot add [alias] [flags]
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-H, --host` | string | empty | Server host name or IP address. |
| `-P, --port` | int | `22` | SSH port. |
| `-u, --user` | string | empty | SSH username. |
| `-p, --password` | string | empty | SSH password. Prefer interactive input when possible. |
| `-k, --key` | string | empty | Managed key alias to use for key authentication. |
| `--auth-method` | string | empty | Authentication method: `password`, `key`, or `agent`. |
| `--known-hosts` | string | empty | Known hosts file path for this server. |
| `-J, --jump-host` | string | empty | Comma-separated jump host alias chain. |
| `--proxy` | string | empty | Managed proxy alias. |
| `-t, --tags` | string | empty | Comma-separated server tags. |

Examples:

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
knot add db --host 10.0.0.5 --user root --jump-host bastion
```

When host and user are provided, the command runs non-interactively. Otherwise it starts a guided prompt.

## Edit a Server

```sh
knot edit [alias] [flags]
```

Most options match `knot add`, with these common edit-specific operations:

| Flag | Description |
| --- | --- |
| `--alias` | Rename the server alias. |
| `-J, --jump-host` | Set jump hosts. Passing an explicitly changed empty value clears them. |
| `--proxy` | Set a managed proxy. Passing an explicitly changed empty value clears it. |
| `-t, --tags` | Set tags. Passing an explicitly changed empty value clears them. |

Examples:

```sh
knot edit web-prod --host 1.2.3.5
knot edit web-prod --alias web-blue --tags prod,blue
```

If no edit flags are provided, Knot opens an interactive editor.

## List and Remove Servers

```sh
knot list [pattern]
knot remove [alias]
```

`knot list` has the alias `knot ls` and can filter by alias, user, host, or tags with case-insensitive matching.

`knot remove` has the aliases `knot rm` and `knot delete`.

```sh
knot list prod
knot rm old-host
```

## Jump Hosts and Proxy

Server profiles can describe both jump hosts and proxy paths:

```sh
knot add app --host 10.0.1.20 --user deploy --jump-host bastion --proxy corp
```

- `--jump-host` references existing server aliases. Multiple jump hosts are comma-separated.
- `--proxy` references a managed proxy from [Proxy](/features/proxy).
