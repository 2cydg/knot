# Port Forwarding

`knot forward` manages local, remote, and dynamic port forwarding rules. Rules are owned by the daemon and can use existing SSH connection paths.

## Forward Types

| Type | Flag | Purpose |
| --- | --- | --- |
| Local | `-L localPort:remoteAddr` | Access a remote-network service from a local port. |
| Remote | `-R remotePort:localAddr` | Expose a local-network service from a remote port. |
| Dynamic | `-D localPort` | Start a local dynamic SOCKS5 forward. |

## Add a Rule

```sh
knot forward add [alias] [flags]
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-L, --local` | string | empty | Local forward in `localPort:remoteAddr` form. |
| `-R, --remote` | string | empty | Remote forward in `remotePort:localAddr` form. |
| `-D, --dynamic` | int | `0` | Dynamic SOCKS5 forward local port. |
| `-t, --temp` | bool | `false` | Create a temporary rule that is not saved to config. |

Examples:

```sh
knot forward add web-prod -L 8080:127.0.0.1:80
knot forward add web-prod -D 1080 --temp
```

Without forwarding flags, Knot starts an interactive creation flow.

## List Rules

```sh
knot forward list [alias]
knot forward ls web-prod
```

Output includes alias, type, port, target address, temporary status, runtime status, and error. Use `--json` for structured output.

## Enable, Disable, and Remove

```sh
knot forward enable [alias] [type:port]
knot forward disable [alias] [type:port]
knot forward remove [alias] [type:port]
```

`type:port` looks like `L:8080`, `R:9000`, or `D:1080`. If omitted, Knot asks you to choose a rule interactively.

`knot forward remove` has the alias `knot forward rm`.

Examples:

```sh
knot forward enable web-prod L:8080
knot forward disable web-prod L:8080
knot forward rm web-prod
```
