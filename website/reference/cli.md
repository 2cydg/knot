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

Every completion command supports:

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--no-descriptions` | bool | `false` | Disable completion descriptions. |

Examples:

```sh
source <(knot completion bash)
knot completion bash > ~/.local/share/bash-completion/completions/knot
autoload -U compinit && compinit && source <(knot completion zsh)
knot completion fish | source
```

PowerShell:

```powershell
knot completion powershell | Out-String | Invoke-Expression
```

## Version

```sh
knot version
knot version --json
```

`knot version` shows the Knot version, commit, build date, operating system, and architecture.

The root command also supports:

```sh
knot --version
```
