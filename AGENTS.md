# AGENTS.md

## Project Philosophy

Knot is a minimalist SSH/SFTP manager built for native terminals, scripts, and AI agents. It does not provide a TUI and does not try to replace the terminal emulator users already prefer. Instead, it brings server profiles, authentication, jump hosts, proxies, file transfer, remote execution, and port forwarding into one CLI workflow.

The project favors simple command-line interaction, persistent SSH connection reuse through a background daemon, platform-backed encryption for sensitive data, and output/error semantics that work well in scripts and automation.

## Core Logic

- `cmd/knot/commands` is the CLI layer. It uses Cobra for commands, flags, completion, and output formatting. The root command rewrites an unknown first argument into `knot ssh <alias>`, so `knot web-prod` is the shortcut for connecting to an alias.
- `pkg/daemon` is the background process layer. It receives CLI requests over a local socket and owns SSH sessions, remote execution, SFTP, port forwarding, status queries, and connection cleanup. The CLI can auto-start the daemon when needed.
- `internal/protocol` defines the binary protocol between the CLI and daemon. The header is 8 bytes: magic, version, message type, subtype/reserved, and payload length. Most business payloads are JSON.
- `pkg/sshpool` manages the reusable `ssh.Client` pool. Pool keys are based on stable server IDs and connection parameters, with support for jump chains, proxies, keepalive, idle cleanup, and reference counting.
- `pkg/config` handles TOML config loading/saving, alias lookup, ID generation, and secret processing. Passwords, private keys, and proxy passwords are stored encrypted with the `ENC:` prefix.
- `pkg/sftp` contains the interactive SFTP REPL, path parsing, completion cache, and upload/download logic. `knot cp` uses Docker-style `alias:/path` syntax for local and remote copies.

## Collaboration Rules

- Do not run `git commit` until the user explicitly asks to commit code.
- When committing code, the commit message must follow Conventional Commits, for example `feat: add sftp batch command` or `fix: handle stale daemon socket`.
- Keep Go code idiomatic, follow the existing package boundaries and style, and avoid unrelated refactors.
- Avoid adding new dependencies unless they are necessary. Prefer the Go standard library first, then third-party packages only when the benefit is clear, because binary size matters for this project.
- When modifying older code that lacks test coverage, add focused unit tests or regression tests where practical.
- After core logic changes, prefer running `go test ./...`; for release or build-related changes, also confirm `go build -o knot cmd/knot/main.go` succeeds.

## Common Commands

```bash
go test ./...
go build -o knot cmd/knot/main.go
go mod tidy
```
