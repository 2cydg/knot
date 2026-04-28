# Remote Exec

`knot exec` runs a non-interactive command on a remote server. It is suitable for scripts, CI, and AI agents because the remote exit code is preserved and output can be rendered as JSON.

## `knot exec`

```sh
knot exec [alias] [command...] [flags]
```

| Argument | Required | Description |
| --- | --- | --- |
| `[alias]` | Yes | Server alias. |
| `[command...]` | Yes | Remote command and arguments. Knot joins them with spaces. |

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-t, --timeout` | int | `60` | Command timeout in seconds. Use `0` for no timeout. |
| `--json` | bool | `false` | Output structured JSON. |

Examples:

```sh
knot exec web-prod "uptime"
knot exec web-prod "systemctl status nginx" --json
knot exec web-prod "tail -n 100 /var/log/nginx/error.log" --timeout 10
```

## Automation Notes

- Use `--json` when another program needs to read the output.
- Set `--timeout 0` or a larger timeout for long-running tasks.
- For complex shell logic, pass one quoted remote command, for example `knot exec web "sh -lc 'cd /srv/app && git status'"`.
