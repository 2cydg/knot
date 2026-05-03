# Knot

[简体中文](./README_zh.md) | English

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue.svg)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-lightgrey.svg)](#)

**Knot** is a CLI-based SSH/SFTP client.

Full documentation: [https://knot.clay.li](https://knot.clay.li)

---

## Why Knot?

*   🔁 **SSH connection reuse**
*   📡 **SSH command broadcast**
*   📂 **SFTP directory following**
*   🌉 **Jump hosts, proxies, and port forwarding**
*   🔑 **SSH Agent authentication and forwarding**
*   🔐 **Platform-native credential encryption**
*   🧾 **AI-friendly JSON output with exit codes**
*   🪶 **Low memory footprint**

---

## Installation

Linux/macOS:

```bash
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

By default, Knot is installed to `~/.local/bin`. Make sure that directory is in your `PATH`.

Build from source if you prefer a local build (requires Go 1.20+):

```bash
go build -o knot cmd/knot/main.go
# Install for the current user
mkdir -p ~/.local/bin
mv knot ~/.local/bin/
# Ensure ~/.local/bin is in your PATH
```

### Shell Completion

Knot provides built-in completion for Bash, Zsh, Fish, and PowerShell.

#### Bash

```bash
# Enable for the current session
source <(knot completion bash)

# Enable permanently for the current user
mkdir -p ~/.local/share/bash-completion/completions && knot completion bash > ~/.local/share/bash-completion/completions/knot
```

Make sure `bash-completion` is installed and loaded by your shell.

#### Zsh

```bash
# Enable for the current session
autoload -U compinit && compinit && source <(knot completion zsh)

# Enable permanently for the current user
mkdir -p ~/.zfunc && knot completion zsh > ~/.zfunc/_knot && grep -qxF 'fpath=("$HOME/.zfunc" $fpath)' ~/.zshrc || printf '\nfpath=("$HOME/.zfunc" $fpath)\nautoload -U compinit && compinit\n' >> ~/.zshrc
```

#### Fish

```bash
# Enable for the current session
knot completion fish | source

# Enable permanently for the current user
mkdir -p ~/.config/fish/completions && knot completion fish > ~/.config/fish/completions/knot.fish
```

#### PowerShell

```powershell
# Enable for the current session
knot completion powershell | Out-String | Invoke-Expression

# Enable permanently for the current user
if (!(Test-Path $PROFILE)) { New-Item -ItemType File -Force $PROFILE | Out-Null }; if (-not (Select-String -Path $PROFILE -SimpleMatch 'knot completion powershell | Out-String | Invoke-Expression' -Quiet -ErrorAction SilentlyContinue)) { Add-Content -Path $PROFILE -Value "`nknot completion powershell | Out-String | Invoke-Expression" }
```

---

## Quick Start

### 1. Add a Server
Knot will guide you through the setup or you can use flags for automation.
```bash
knot add web-prod --host 1.2.3.4 --user deploy --key my_key --tags prod
```

### 2. Connect
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

### 5. Follow an SSH Session from SFTP
```bash
# In one terminal
knot web-prod

# In another terminal, follow that SSH session's current directory
knot sftp web-prod --follow
```

### 6. Broadcast Input Across SSH Sessions
```bash
knot ssh web-1 --broadcast deploy
knot ssh web-2 --broadcast deploy

knot broadcast list
knot broadcast pause web-2
```

### 7. Remote Execution
```bash
knot exec web-prod "uptime" --json
```

For automation, `--json` keeps failure exit codes meaningful while returning machine-readable output. Use `--host-key-policy accept-new` or `fail` when host key prompts must be controlled explicitly.

---

## Architecture

Knot uses a Client/Server model to maintain persistent SSH connections in the background.

<img width="511" height="241" alt="PixPin_2026-04-21_17-22-20" src="https://github.com/user-attachments/assets/caca981a-644c-456f-8f1a-59cc119fc87b" />


*   **Daemon**: Maintains a pool of physical SSH connections.
*   **CLI**: Lightweight frontend that talks to the daemon over UDS.
*   **Protocol**: Compact 8-byte header binary protocol for low-latency IPC.

---

## Security

Sensitive data in `~/.config/knot/config.toml` is encrypted with an `ENC:` prefix:
- **Windows**: DPAPI
- **macOS**: Keychain
- **Linux**: AES-256-GCM (Derived from `/etc/machine-id` + Salt)

Default filesystem layout:
- Config: `~/.config/knot/`
- Known hosts: `~/.config/knot/known_hosts`
- Logs and state: `~/.local/state/knot/`
- Runtime files (`sock`, `pid`): `$XDG_RUNTIME_DIR/knot/`

Knot stores host keys in its own `known_hosts` file. Non-interactive commands can control host key behavior with `--host-key-policy` when needed.

---

## Command Reference

| Category | Command | Description |
| :--- | :--- | :--- |
| **Sessions** | `knot [alias]` | Shortcut for `knot ssh [alias]` |
| | `knot ssh [alias] --broadcast [group]` | Join an interactive SSH input broadcast group |
| | `knot broadcast list/show/pause/resume/leave/disband` | Inspect and manage broadcast groups |
| | `knot sftp [alias]` | Interactive SFTP shell |
| | `knot sftp [alias] --follow` | Follow the current directory of an active SSH session |
| | `knot sftp ls/stat/rm/mkdir/rmdir/mv` | Scriptable SFTP operations |
| **Files** | `knot cp [src] [dst]` | File transfer (Local ↔ Remote) |
| **Remote** | `knot exec [alias] [cmd]` | Non-interactive command execution |
| **Network** | `knot forward` | Manage L/R/D port forwarding rules |
| **Manager** | `knot list [pattern]` | List servers by alias, target, tags, and recent usage |
| | `knot status` | Check daemon and connection pool health |
| | `knot export/import` | Encrypted configuration backup |

---

## License

This project is licensed under the [MIT License](LICENSE).
