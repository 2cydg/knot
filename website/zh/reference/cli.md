# Shell 补全与版本

## Shell 补全

```sh
knot completion [command]
```

| 命令 | 说明 |
| --- | --- |
| `knot completion bash` | 生成 Bash 补全。 |
| `knot completion zsh` | 生成 Zsh 补全。 |
| `knot completion fish` | 生成 Fish 补全。 |
| `knot completion powershell` | 生成 PowerShell 补全。 |

每个补全命令都支持：

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--no-descriptions` | bool | `false` | 禁用补全描述。 |

示例：

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

## 版本信息

```sh
knot version
knot version --json
```

`knot version` 显示 Knot 版本、commit、构建日期、操作系统和架构。

根命令也支持：

```sh
knot --version
```
