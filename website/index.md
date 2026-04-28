# Knot Documentation

Knot is a native-terminal SSH/SFTP manager. It does not provide a TUI and does not replace the terminal emulator you already use. It brings server profiles, authentication, jump hosts, proxies, file transfer, remote execution, port forwarding, and connection reuse into one CLI workflow.

```sh
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

## Daily Workflow

<div class="quick-grid">
  <div class="quick-card">
    <strong>SSH</strong>
    <p>Save a server alias, then use <code>knot ssh alias</code> or the shortcut <code>knot alias</code> to open a remote shell.</p>
  </div>
  <div class="quick-card">
    <strong>SFTP</strong>
    <p>Use the interactive SFTP shell, or run direct operations with <code>alias:/path</code> remote paths.</p>
  </div>
  <div class="quick-card">
    <strong>Proxy and Jump Hosts</strong>
    <p>Server profiles can reference managed proxies and comma-separated jump host chains.</p>
  </div>
  <div class="quick-card">
    <strong>Daemon</strong>
    <p>The background daemon owns the SSH connection pool so SSH, exec, SFTP, cp, and forward operations can reuse connections.</p>
  </div>
</div>

## Quick Example

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
knot web-prod
knot exec web-prod "uptime" --json
knot cp ./dist/. web-prod:/var/www/html/
```

## Documentation Structure

- [Install and Quick Start](/guide/getting-started): install Knot, add a server, connect, and copy files.
- [SSH Connections](/features/ssh): server profiles, connection entry points, shortcuts, and jump hosts.
- [SFTP and File Copy](/features/sftp): interactive SFTP, batch SFTP commands, and `knot cp`.
- [Proxy](/features/proxy): managed HTTP/SOCKS5 proxies and server references.
- [Port Forwarding](/features/forward): local, remote, and dynamic SOCKS5 forwarding.
- [Servers and Global Config](/features/config): server CRUD, global settings, and config import/export.
