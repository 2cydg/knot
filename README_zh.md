# Knot

简体中文 | [English](./README.md)

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey.svg)](#)

**Knot** 是一款极简、高性能的 SSH/SFTP 管理工具，专为开发者与 AI Agent 设计。通过后台守护进程 (Daemon) 与连接复用技术，它彻底消除了传统 SSH 的握手开销，让远程会话和文件传输几乎达到“瞬时”响应。

---

## 🚀 为什么选择 Knot?

*   ⚡ **瞬时连接**: 基于连接复用技术，后台维护物理连接，新建会话无需等待握手。
*   🔌 **高级网络支持**: 内置跳板机链 (Jump Host) 与 SOCKS5/HTTP 代理支持。
*   🔒 **原生安全**: 密码与密钥绝不以明文存储。深度集成系统级加密（Windows DPAPI, macOS Keychain, Linux Secret Service 及其 Machine-ID 降级方案）。
*   🤖 **AI & 脚本友好**: 所有命令原生支持 `--json` 输出，提供完整的非交互模式，完美适配自动化脚本。
*   🛠️ **现代 SFTP**: 交互式 REPL 环境，高效管理远程文件。
*   🔌 **强力转发**: 轻松管理本地 (L)、远程 (R) 和动态 (D/SOCKS5) 端口转发。

---

## 📦 安装

从源码编译（需要 Go 1.20+）：

```bash
go build -o knot cmd/knot/main.go
# 安装到当前用户目录
mkdir -p ~/.local/bin
mv knot ~/.local/bin/
# 确保 ~/.local/bin 已加入 PATH
```

### Shell 补全

Knot 内置了对 Bash, Zsh, 和 Fish 的补全支持。

```bash
# Zsh 示例
knot completion zsh > ~/.zfunc/_knot
# Bash 示例
knot completion bash > /etc/bash_completion.d/knot
```

---

## 🛠️ 快速上手

### 1. 添加服务器
Knot 提供交互式引导，也支持通过参数快速添加。
```bash
knot add web-prod --host 1.2.3.4 --user deploy --key my_key --tags prod
```

### 2. 瞬时连接
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

### 5. 远程命令执行
```bash
knot exec web-prod "uptime" --json
```

---

## 🏗️ 架构设计

Knot 采用 C/S 模型，在后台维护持久的 SSH 连接。

<img width="511" height="241" alt="PixPin_2026-04-21_17-22-20" src="https://github.com/user-attachments/assets/caca981a-644c-456f-8f1a-59cc119fc87b" />

*   **Daemon**: 维护物理 SSH 连接池。
*   **CLI**: 轻量级前端，通过 UDS 与守护进程通信。
*   **协议**: 紧凑的 8 字节头部二进制协议，确保低延迟。

---

## 🔒 安全性

`~/.config/knot/config.toml` 中的敏感数据均带有 `ENC:` 前缀进行加密存储：
- **Windows**: DPAPI
- **macOS**: Keychain
- **Linux**: AES-256-GCM (密钥存储于 Secret Service / D-Bus，降级方案为 `/etc/machine-id` + 盐值)

默认目录布局：
- 配置：`~/.config/knot/`
- `known_hosts`：`~/.config/knot/known_hosts`
- 日志与状态：`~/.local/state/knot/`
- 运行时文件（`sock`、`pid`）：`$XDG_RUNTIME_DIR/knot/`

---

## ⌨️ 常用命令参考

| 分类 | 命令 | 说明 |
| :--- | :--- | :--- |
| **会话管理** | `knot [别名]` | `knot ssh [别名]` 的快捷方式 |
| | `knot sftp [alias]` | 交互式 SFTP Shell |
| **文件操作** | `knot cp [源] [目标]` | 高速文件传输（本地 ↔ 远程） |
| **远程执行** | `knot exec [别名] [命令]` | 非交互式远程命令执行 |
| **网络功能** | `knot forward` | 管理 L/R/D 端口转发规则 |
| **管理工具** | `knot list [模式]` | 查看服务器别名、目标地址、标签和最近使用情况 |
| | `knot status` | 查看守护进程与连接池状态 |
| | `knot export/import` | 加密后的配置备份与导入 |

---

## 📄 许可证

本项目采用 [MIT License](LICENSE) 许可证。
