# 安装与快速上手

## 安装

Linux 和 macOS 使用 shell 安装脚本：

```sh
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows 使用 PowerShell 安装脚本：

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

从源码构建：

```sh
go build -o knot ./cmd/knot
mkdir -p ~/.local/bin
mv knot ~/.local/bin/
```

## 推荐：启用 Shell 补全

建议安装后先启用补全。Knot 的命令、子命令、服务器别名和部分参数都依赖补全提升效率；没有补全时，日常 SSH、SFTP、复制和转发操作会明显更慢。

### Bash

当前 shell 临时启用：

```sh
source <(knot completion bash)
```

持久启用：

```sh
mkdir -p ~/.local/share/bash-completion/completions
knot completion bash > ~/.local/share/bash-completion/completions/knot
```

### Zsh

当前 shell 临时启用：

```sh
autoload -U compinit && compinit
source <(knot completion zsh)
```

持久启用：

```sh
mkdir -p ~/.zfunc
knot completion zsh > ~/.zfunc/_knot
printf '\nfpath=(~/.zfunc $fpath)\nautoload -U compinit\ncompinit\n' >> ~/.zshrc
```

### Fish

当前 shell 临时启用：

```sh
knot completion fish | source
```

持久启用：

```sh
mkdir -p ~/.config/fish/completions
knot completion fish > ~/.config/fish/completions/knot.fish
```

### PowerShell

当前 shell 临时启用：

```powershell
knot completion powershell | Out-String | Invoke-Expression
```

持久启用：

```powershell
New-Item -Type File -Path $PROFILE -Force
Add-Content -Path $PROFILE -Value 'knot completion powershell | Out-String | Invoke-Expression'
```

更多细节见 [Shell 补全与版本](/zh/reference/cli)。

## 创建第一个服务器

最常见的非交互写法：

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
```

如果不提供完整参数，`knot add` 会进入交互式配置流程：

```sh
knot add web-prod
```

## 连接

完整命令是：

```sh
knot ssh web-prod
```

根命令会把未知的第一个参数重写成 SSH 连接，因此日常可以直接写：

```sh
knot web-prod
```

这个快捷写法只属于 `knot ssh [alias]`，不会单独作为一个命令文档维护。

## 远程执行

```sh
knot exec web-prod "uptime"
knot exec web-prod "systemctl status nginx" --json
```

`knot exec` 会保留远程命令退出码，适合脚本和自动化。

## 文件传输

```sh
knot cp ./dist/. web-prod:/var/www/html/
knot cp web-prod:/var/log/nginx/access.log ./
```

远程路径使用 `alias:/path` 形式。源目录以 `/.` 结尾时，复制的是目录内容。

## 常用全局选项

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `--json` | `false` | 输出 JSON，适合脚本和自动化。 |
| `--host-key-policy` | 空 | 主机密钥策略：`fail`、`accept-new`、`strict`、`insecure-skip`。 |
| `-h, --help` | `false` | 查看当前命令帮助。 |
