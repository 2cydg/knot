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

Bash、Zsh 和 Fish 支持：

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--no-descriptions` | bool | `false` | 禁用补全描述。 |

PowerShell 默认为无描述补全，以降低 shell 启动成本。它支持：

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--descriptions` | bool | `false` | 启用补全描述。 |

示例：

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

## Sync 命令

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

WebDAV provider 选项：

| 选项 | 类型 | 说明 |
| --- | --- | --- |
| `--url` | string | WebDAV 对象 URL 或目录 URL。 |
| `--user` | string | WebDAV 用户名。 |
| `--password` | string | WebDAV 密码，会加密保存在本机。 |

S3 provider add/edit 选项：

| 选项 | 类型 | 说明 |
| --- | --- | --- |
| `--bucket` | string | S3 bucket，新增时必填。 |
| `--key` | string | S3 object key，默认 `config.toml.enc`。 |
| `--region` | string | S3 签名 region，新增时必填。 |
| `--endpoint` | string | 可选 S3-compatible endpoint URL。 |
| `--access-key-id` | string | S3 access key ID，会加密保存在本机。 |
| `--secret-access-key` | string | S3 secret access key，会加密保存在本机。 |
| `--session-token` | string | 可选 S3 session token，会加密保存在本机。编辑时传 `-` 可清空。 |
| `--path-style` | bool | 对要求 bucket 放在路径里的兼容 endpoint 使用 S3 path-style URL。 |

`provider show` 不会输出 provider 敏感信息。`provider list` 中 S3 target 显示为 `s3://bucket/key`。

## 版本信息

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

`knot version` 显示 Knot 版本、commit、构建日期、操作系统和架构。

`knot version check` 会读取最新 stable release manifest，并报告是否有可用更新。JSON 输出使用全局 `--json` 选项。

`knot version upgrade` 会先检查版本，只在发现新版本时升级。`knot upgrade` 是同一操作的快捷命令。

如果 daemon 中有活跃 SSH 会话，升级会在停止 daemon 前确认，因为这些会话会被断开。非交互脚本中需要显式传入 `-y` 或 `--yes`；否则存在活跃会话时命令会拒绝继续。

开发构建（`version=dev`）不支持自升级。命令会提示不支持并以成功状态退出。

根命令也支持：

```sh
knot --version
```
