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
- Always respect `.gitignore`; do not force-add, stage, or otherwise bypass ignored paths unless the user explicitly asks.
- Keep Go code idiomatic, follow the existing package boundaries and style, and avoid unrelated refactors.
- For terminal-facing SSH features, preserve the byte stream from the remote server to the local terminal as faithfully as possible. Avoid filtering, buffering, rewriting, or interpreting PTY output unless the behavior is explicitly required and covered by focused regression tests.
- Avoid adding new dependencies unless they are necessary. Prefer the Go standard library first, then third-party packages only when the benefit is clear, because binary size matters for this project.
- When modifying older code that lacks test coverage, add focused unit tests or regression tests where practical.
- When adding tests, pay special attention to Windows compatibility: use `filepath` for local paths, avoid Unix-only assumptions, and consider Windows CI behavior for sockets, terminals, file locking, and platform-backed crypto.
- Do not let tests depend on live platform credential stores such as macOS Keychain, Windows DPAPI, or Linux Secret Service when asserting encrypted config behavior; inject a deterministic test crypto provider so CI can decrypt data across repeated loads.
- After core logic changes, prefer running `go test ./...`; for release or build-related changes, also confirm `go build -o knot cmd/knot/main.go` succeeds.

## Workflow Automation

### PR Creation and Merge

When the user says "pr", "create pr", "submit pr", or similar, the Agent automatically:

1. `git push -u origin HEAD` — push the current branch
2. `gh pr create` — create PR with title and description
3. `gh pr checks --watch` — wait for CI checks to pass
4. `gh pr merge --squash --delete-branch` — squash merge and delete branch

If `gh` authentication fails, the Agent generates PR info (link, title, body) and provides commands for the user to execute manually.

### Version Upgrade and Release

When the user says "upgrade", "release", "tag", or similar:

1. **Version number**: use user-specified version if provided; otherwise determine from code changes:
   - Breaking changes or major features → bump major version
   - New features (backward compatible) → bump minor version
   - Bug fixes → bump patch version
2. **Diff check**: `git diff main...HEAD` or `git log` to review unmerged commits
3. **Release Note**: bilingual, Chinese first then English, one item per line:
   ```
   feat: feature description
   fix: fix description
   refactor: refactor description
   ...

   feature description in English
   fix description in English
   refactor description in English
   ```
4. **Tag push**: attempt `git tag v{x.y.z} && git push origin v{x.y.z}`; if auth fails, provide commands for the user

## Common Commands

```bash
go test ./...
go build -o knot cmd/knot/main.go
go mod tidy
```
