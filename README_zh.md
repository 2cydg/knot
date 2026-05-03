# Knot

简体中文 | [English](./README.md)

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey.svg)](#)

**Knot** 是一个基于命令行的 SSH/SFTP 客户端。

详细文档：[https://knot.clay.li](https://knot.clay.li)

---

## 为什么选择 Knot?

*   🔁 **SSH连接复用**
*   📡 **SSH命令广播**
*   📂 **SFTP 目录跟随**
*   🌉 **跳板机、代理、端口转发**
*   🔑 **SSH Agent 认证与转发**
*   🔐 **平台原生凭据加密**
*   🧾 **AI友好的JSON 输出，保留退出码**
*   🪶 **低内存占用**

---

## 安装

Linux/macOS：

```bash
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

默认安装到 `~/.local/bin`。请确保该目录已加入 `PATH`。

如需本地构建，也可以从源码编译（需要 Go 1.20+）：

```bash
go build -o knot cmd/knot/main.go
# 安装到当前用户目录
mkdir -p ~/.local/bin
mv knot ~/.local/bin/
# 确保 ~/.local/bin 已加入 PATH
```

### Shell 补全

Knot 内置了对 Bash、Zsh、Fish 和 PowerShell 的补全支持。

#### Bash

```bash
# 当前会话立即启用
source <(knot completion bash)

# 为当前用户永久启用
mkdir -p ~/.local/share/bash-completion/completions && knot completion bash > ~/.local/share/bash-completion/completions/knot
```

请确认系统已安装并加载 `bash-completion`。

#### Zsh

```bash
# 当前会话立即启用
autoload -U compinit && compinit && source <(knot completion zsh)

# 为当前用户永久启用
mkdir -p ~/.zfunc && knot completion zsh > ~/.zfunc/_knot && grep -qxF 'fpath=("$HOME/.zfunc" $fpath)' ~/.zshrc || printf '\nfpath=("$HOME/.zfunc" $fpath)\nautoload -U compinit && compinit\n' >> ~/.zshrc
```

#### Fish

```bash
# 当前会话立即启用
knot completion fish | source

# 为当前用户永久启用
mkdir -p ~/.config/fish/completions && knot completion fish > ~/.config/fish/completions/knot.fish
```

#### PowerShell

```powershell
# 当前会话立即启用
knot completion powershell | Out-String | Invoke-Expression

# 为当前用户永久启用
if (!(Test-Path $PROFILE)) { New-Item -ItemType File -Force $PROFILE | Out-Null }; if (-not (Select-String -Path $PROFILE -SimpleMatch 'knot completion powershell | Out-String | Invoke-Expression' -Quiet -ErrorAction SilentlyContinue)) { Add-Content -Path $PROFILE -Value "`nknot completion powershell | Out-String | Invoke-Expression" }
```

---

## 快速上手

### 1. 添加服务器
Knot 提供交互式引导，也支持通过参数快速添加。
```bash
knot add web-prod --host 1.2.3.4 --user deploy --key my_key --tags prod
```

### 2. 连接
未识别的子命令会被自动作为别名处理。只需输入 `knot [别名]` 即可。
```bash
knot web-prod
```

### 3. 跳板机与代理
```bash
# 添加代理
knot proxy add my-socks5 --host 127.0.0.1 --port 1080 --type socks5

# 添加带代理或跳板机链的服务器
knot add web-prod --host 1.2.3.4 --proxy my-socks5
knot add db-internal --host 10.0.0.5 --jump jumphost1,jumphost2
```

### 4. 文件传输 (Docker 风格)
```bash
# 上传
knot cp ./dist/. web-prod:/var/www/html/
# 下载
knot cp web-prod:/var/log/nginx/access.log ./
```

### 5. 从 SFTP 跟随 SSH 会话目录
```bash
# 在一个终端中
knot web-prod

# 在另一个终端中，跟随该 SSH 会话的当前目录
knot sftp web-prod --follow
```

### 6. 在多个 SSH 会话间广播输入
```bash
knot ssh web-1 --broadcast deploy
knot ssh web-2 --broadcast deploy

knot broadcast list
knot broadcast pause web-2
```

### 7. 远程命令执行
```bash
knot exec web-prod "uptime" --json
```

自动化场景下，`--json` 只改变输出格式，不吞掉失败退出码。需要显式控制 host key 提示时，可以使用 `--host-key-policy accept-new` 或 `fail`。

---

## 架构设计

Knot 采用 C/S 模型，在后台维护持久的 SSH 连接。

<img width="511" height="241" alt="PixPin_2026-04-21_17-22-20" src="https://github.com/user-attachments/assets/caca981a-644c-456f-8f1a-59cc119fc87b" />

*   **Daemon**: 维护物理 SSH 连接池。
*   **CLI**: 轻量级前端，通过 UDS 与守护进程通信。
*   **协议**: 紧凑的 8 字节头部二进制协议，确保低延迟。

---

## 安全性

`~/.config/knot/config.toml` 中的敏感数据均带有 `ENC:` 前缀进行加密存储：
- **Windows**: DPAPI
- **macOS**: Keychain
- **Linux**: AES-256-GCM (密钥存储于 Secret Service / D-Bus，降级方案为 `/etc/machine-id` + 盐值)

默认目录布局：
- 配置：`~/.config/knot/`
- `known_hosts`：`~/.config/knot/known_hosts`
- 日志与状态：`~/.local/state/knot/`
- 运行时文件（`sock`、`pid`）：`$XDG_RUNTIME_DIR/knot/`

Knot 使用自己的 `known_hosts` 文件保存主机密钥。非交互命令可按需通过 `--host-key-policy` 控制 host key 行为。

---

## 常用命令参考

| 分类 | 命令 | 说明 |
| :--- | :--- | :--- |
| **会话管理** | `knot [别名]` | `knot ssh [别名]` 的快捷方式 |
| | `knot ssh [别名] --broadcast [组名]` | 加入交互式 SSH 输入广播组 |
| | `knot broadcast list/show/pause/resume/leave/disband` | 查看和管理广播组 |
| | `knot sftp [alias]` | 交互式 SFTP Shell |
| | `knot sftp [alias] --follow` | 跟随活动 SSH 会话的当前目录 |
| | `knot sftp ls/stat/rm/mkdir/rmdir/mv` | 脚本化 SFTP 操作 |
| **文件操作** | `knot cp [源] [目标]` | 文件传输（本地 ↔ 远程） |
| **远程执行** | `knot exec [别名] [命令]` | 非交互式远程命令执行 |
| **网络功能** | `knot forward` | 管理 L/R/D 端口转发规则 |
| **管理工具** | `knot list [模式]` | 查看服务器别名、目标地址、标签和最近使用情况 |
| | `knot status` | 查看守护进程与连接池状态 |
| | `knot export/import` | 加密后的配置备份与导入 |

---

## 许可证

本项目采用 [MIT License](LICENSE) 许可证。
