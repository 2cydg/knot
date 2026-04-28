# Daemon and Status

Knot's background daemon owns persistent SSH connections, remote execution, SFTP, port forwarding, status queries, and connection cleanup. The CLI can auto-start the daemon when needed.

## Daemon Commands

```sh
knot daemon [command]
```

| Command | Description |
| --- | --- |
| `knot daemon start` | Start the daemon. Hidden shortcut: `knot start`. |
| `knot daemon stop` | Stop the daemon. Hidden shortcut: `knot stop`. |
| `knot daemon restart` | Restart the daemon. Hidden shortcut: `knot restart`. |
| `knot daemon clear` | Disconnect all active SSH connections held by the daemon. Hidden shortcut: `knot clear`. |

`knot daemon start` supports:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-f, --foreground` | bool | `false` | Run the daemon in the foreground. |

Examples:

```sh
knot daemon start
knot daemon start --foreground
knot daemon clear
```

## Status

```sh
knot status
knot status --json
```

`knot status` shows daemon status and connection pool statistics. Use `--json` for scripts.

## Logs

```sh
knot logs [flags]
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-t, --tail` | int | `100` | Number of lines to show from the end of the log file. |
| `-f, --follow` | bool | `false` | Follow log output. |

Examples:

```sh
knot logs
knot logs --tail=50
knot logs --tail=20 -f
```
