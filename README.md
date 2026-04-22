# Knot

[简体中文](./README_zh.md) | English

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey.svg)](#)

**Knot** is a minimalist, high-performance SSH/SFTP management tool designed for developers and AI Agents. By utilizing a background daemon and connection multiplexing, it eliminates the handshake overhead of traditional SSH, making remote sessions and file transfers nearly instant.

---

## 🚀 Why Knot?

*   ⚡ **Instant Connectivity**: Connection multiplexing via a background daemon. No more waiting for SSH handshakes.
*   🔒 **Native Security**: Passwords and keys are never stored in plaintext. Uses OS-level encryption (Windows DPAPI, macOS Keychain, Linux Machine-ID).
*   🤖 **AI & Scripting Ready**: Built-in `--json` support for all commands and non-interactive modes for seamless automation.
*   🛠️ **Modern SFTP**: Interactive REPL for efficient remote file management.
*   🔌 **Powerful Forwarding**: Easy management of local, remote, and dynamic (SOCKS5) port forwarding.

---

## 📦 Installation

Build from source (requires Go 1.20+):

```bash
go build -o knot cmd/knot/main.go
# Move to your PATH, e.g., /usr/local/bin/
sudo mv knot /usr/local/bin/
```

### Shell Completion

Knot provides built-in completion for Bash, Zsh, and Fish.

```bash
# For Zsh
knot completion zsh > ~/.zfunc/_knot
# For Bash
knot completion bash > /etc/bash_completion.d/knot
```

---

## 🛠️ Quick Start

### 1. Add a Server
Knot will guide you through the setup or you can use flags for automation.
```bash
knot add web-prod --host 1.2.3.4 --user deploy --key my_key --tags prod
```

### 2. Connect Instantly
Unknown subcommands are treated as aliases. `knot [alias]` is all you need.
```bash
knot web-prod
```

### 3. Jump Hosts & Proxies
```bash
# Add a proxy
knot proxy add my-socks5 --host 127.0.0.1 --port 1080 --type socks5

# Add a server with a proxy or jump host chain
knot add web-prod --host 1.2.3.4 --proxy my-socks5
knot add db-internal --host 10.0.0.5 --jump jumphost1,jumphost2
```

### 4. File Transfer (Docker-style)
```bash
# Upload
knot cp ./dist/. web-prod:/var/www/html/
# Download
knot cp web-prod:/var/log/nginx/access.log ./
```

### 4. Remote Execution
```bash
knot exec web-prod "uptime" --json
```

---

## 🏗️ Architecture

Knot uses a Client/Server model to maintain persistent SSH connections in the background.

<img width="511" height="241" alt="PixPin_2026-04-21_17-22-20" src="https://github.com/user-attachments/assets/caca981a-644c-456f-8f1a-59cc119fc87b" />


*   **Daemon**: Maintains a pool of physical SSH connections.
*   **CLI**: Lightweight frontend that talks to the daemon over UDS.
*   **Protocol**: Compact 8-byte header binary protocol for low-latency IPC.

---

## 🔒 Security

Sensitive data in `~/.config/knot/config.toml` is encrypted with an `ENC:` prefix:
- **Windows**: DPAPI
- **macOS**: Keychain
- **Linux**: AES-256-GCM (Derived from `/etc/machine-id` + Salt)

---

## ⌨️ Command Reference

| Category | Command | Description |
| :--- | :--- | :--- |
| **Sessions** | `knot [alias]` | Shortcut for `knot ssh [alias]` |
| | `knot sftp [alias]` | Interactive SFTP shell |
| **Files** | `knot cp [src] [dst]` | High-speed file transfer (Local ↔ Remote) |
| **Remote** | `knot exec [alias] [cmd]` | Non-interactive command execution |
| **Network** | `knot forward` | Manage L/R/D port forwarding rules |
| **Manager** | `knot list [pattern]` | List servers grouped by tags |
| | `knot status` | Check daemon and connection pool health |
| | `knot export/import` | Encrypted configuration backup |

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
