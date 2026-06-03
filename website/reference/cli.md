# Shell Completion and Version

## Shell Completion

```sh
knot completion [command]
```

| Command | Description |
| --- | --- |
| `knot completion bash` | Generate Bash completion. |
| `knot completion zsh` | Generate Zsh completion. |
| `knot completion fish` | Generate Fish completion. |
| `knot completion powershell` | Generate PowerShell completion. |

Bash, Zsh, and Fish support:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | bool | `false` | Disable completion descriptions. |

PowerShell defaults to no descriptions for faster shell startup. It supports:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--descriptions` | bool | `false` | Enable completion descriptions. |

Examples:

```sh
source <(knot completion bash)
knot completion bash > ~/.local/share/bash-completion/completions/knot
autoload -U compinit && compinit && source <(knot completion zsh)
knot completion fish | source
```

PowerShell:

```powershell
$CompletionDir = Join-Path $HOME ".config\knot"
New-Item -ItemType Directory -Path $CompletionDir -Force | Out-Null
$CompletionPath = Join-Path $CompletionDir "completion.ps1"
knot completion powershell > $CompletionPath
. $CompletionPath
```

## Sync Commands

```sh
knot sync provider add [webdav|s3] [alias]
knot sync provider add webdav [alias]
knot sync provider add s3 [alias]
knot sync provider edit <alias>
knot sync provider list
knot sync provider show <alias>
knot sync push [provider]
knot sync pull [provider] --strategy local-first
```

WebDAV provider flags:

| Flag | Type | Description |
| --- | --- | --- |
| `--url` | string | WebDAV object URL or directory URL. |
| `--user` | string | WebDAV username. |
| `--password` | string | WebDAV password, stored encrypted locally. |

S3 provider add/edit flags:

| Flag | Type | Description |
| --- | --- | --- |
| `--bucket` | string | S3 bucket. Required when adding. |
| `--key` | string | S3 object key. Defaults to `config.toml.enc`. |
| `--region` | string | S3 signing region. Required when adding. |
| `--endpoint` | string | Optional S3-compatible endpoint URL. |
| `--access-key-id` | string | S3 access key ID, stored encrypted locally. |
| `--secret-access-key` | string | S3 secret access key, stored encrypted locally. |
| `--session-token` | string | Optional S3 session token, stored encrypted locally. Use `-` with `provider edit` to clear it. |
| `--path-style` | bool | Use S3 path-style URLs for compatible endpoints that require bucket-in-path addressing. |

`provider show` never prints provider secrets. S3 targets appear in `provider list` as `s3://bucket/key`.

## Version

```sh
knot version
knot version --json
knot version check
knot version check --json
knot version upgrade [-y|--yes]
knot version upgrade [-y|--yes] --json
knot upgrade [-y|--yes]
knot upgrade [-y|--yes] --json
```

`knot version` shows the Knot version, commit, build date, operating system, and architecture.

`knot version check` checks the latest stable release manifest and reports whether an update is available. JSON output uses the global `--json` flag.

`knot version upgrade` checks first, then upgrades only when a newer release exists. `knot upgrade` is the shortcut for the same operation.

If the daemon has active SSH sessions, upgrade asks before stopping the daemon because those sessions will be disconnected. In non-interactive scripts, pass `-y` or `--yes`; otherwise the command refuses to proceed when active sessions exist.

Development builds (`version=dev`) do not self-upgrade. They report that self-upgrade is unsupported and exit successfully.

The root command also supports:

```sh
knot --version
```
