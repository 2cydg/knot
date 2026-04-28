# Proxy

`knot proxy` manages reusable HTTP and SOCKS5 proxies. Server profiles reference proxies with `--proxy`, so proxy addresses and credentials do not need to be repeated on every server.

## Commands

```sh
knot proxy [command]
```

| Command | Description |
| --- | --- |
| `knot proxy list` | List configured proxies. Alias: `knot proxy ls`. |
| `knot proxy add` | Add or overwrite a managed proxy. |
| `knot proxy edit` | Edit a proxy interactively. |
| `knot proxy remove` | Remove a proxy. Alias: `knot proxy rm`. |

## Add a Proxy

```sh
knot proxy add [alias] [flags]
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--type` | string | empty | Proxy type: `socks5` or `http`. |
| `--host` | string | empty | Proxy host. |
| `--port` | int | `0` | Proxy port. |
| `--user` | string | empty | Proxy username. |
| `--password` | string | empty | Proxy password. |

When `--type`, `--host`, and `--port` are provided, the command runs non-interactively:

```sh
knot proxy add corp --type socks5 --host 127.0.0.1 --port 1080
```

## Reference a Proxy from a Server

```sh
knot add app --host 10.0.1.20 --user deploy --proxy corp
knot edit app --proxy corp
```

To clear a server proxy, pass an explicitly changed empty value with `knot edit`.

## Remove a Proxy

```sh
knot proxy remove corp
knot proxy rm corp
```

If servers reference the proxy, Knot asks before clearing those references.
