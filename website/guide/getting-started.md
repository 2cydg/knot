# Install and Quick Start

## Install

Linux and macOS use the shell installer:

```sh
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows uses the PowerShell installer:

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

Build from source:

```sh
go build -o knot ./cmd/knot
mkdir -p ~/.local/bin
mv knot ~/.local/bin/
```

## Recommended: Enable Shell Completion

Enable completion right after installation. Knot completion covers commands, subcommands, server aliases, and some arguments. Without it, daily SSH, SFTP, copy, and forwarding workflows are noticeably slower.

### Bash

Enable it for the current shell:

```sh
source <(knot completion bash)
```

Enable it persistently:

```sh
mkdir -p ~/.local/share/bash-completion/completions
knot completion bash > ~/.local/share/bash-completion/completions/knot
```

### Zsh

Enable it for the current shell:

```sh
autoload -U compinit && compinit
source <(knot completion zsh)
```

Enable it persistently:

```sh
mkdir -p ~/.zfunc
knot completion zsh > ~/.zfunc/_knot
printf '\nfpath=(~/.zfunc $fpath)\nautoload -U compinit\ncompinit\n' >> ~/.zshrc
```

### Fish

Enable it for the current shell:

```sh
knot completion fish | source
```

Enable it persistently:

```sh
mkdir -p ~/.config/fish/completions
knot completion fish > ~/.config/fish/completions/knot.fish
```

### PowerShell

Enable it for the current shell:

```powershell
knot completion powershell | Out-String | Invoke-Expression
```

Enable it persistently:

```powershell
New-Item -Type File -Path $PROFILE -Force
Add-Content -Path $PROFILE -Value 'knot completion powershell | Out-String | Invoke-Expression'
```

See [Shell Completion and Version](/reference/cli) for the command reference.

## Create Your First Server

The most common non-interactive form:

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
```

If you do not provide the full set of parameters, `knot add` starts a guided prompt:

```sh
knot add web-prod
```

## Connect

The full command is:

```sh
knot ssh web-prod
```

The root command rewrites an unknown first argument into an SSH connection, so day to day you can write:

```sh
knot web-prod
```

This shortcut belongs to `knot ssh [alias]`; it is not documented as a separate command.

## Remote Exec

```sh
knot exec web-prod "uptime"
knot exec web-prod "systemctl status nginx" --json
```

`knot exec` preserves the remote command exit code, which makes it suitable for scripts and automation.

## File Transfer

```sh
knot cp ./dist/. web-prod:/var/www/html/
knot cp web-prod:/var/log/nginx/access.log ./
```

Remote paths use the `alias:/path` form. When the source directory ends with `/.`, Knot copies the directory contents.

## Common Global Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Output JSON for scripts and automation. |
| `--host-key-policy` | empty | Host key policy: `fail`, `accept-new`, `strict`, or `insecure-skip`. |
| `-h, --help` | `false` | Show help for the current command. |
