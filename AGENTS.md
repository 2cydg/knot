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

### PR 创建与合并

当用户说"pr"、"创建pr"、"提交pr"或类似指令时，Agent 自动执行以下操作：

1. `git push -u origin HEAD` — 推送当前分支到远程
2. `gh pr create` — 创建 PR，包含标题和描述
3. `gh pr checks --watch` — 等待 CI 检查通过
4. `gh pr merge --squash --delete-branch` — 自动 squash merge 并删除分支

如果 `gh` 认证失败导致无法提交，Agent 生成 PR 信息（链接、标题、内容）并提供命令让用户手动执行。

### 版本升级与 Release

当用户说"升级版本"、"release"、"打版本"或类似指令时：

1. **版本号判定**：用户指定版本号时使用用户指定的；未指定时根据代码变更内容自行判定：
   - 破坏性变更或大功能 → 升级主版本
   - 新功能向后兼容 → 升级副版本
   - bug 修复 → 升级修订版本
2. **差异检查**：`git diff main...HEAD` 或 `git log` 查看未合并的提交
3. **Release Note 生成**：中英文对照，格式如下，每项一行：
   ```
   feat: 新功能描述
   fix: 修复描述
   refactor: 重构描述
   ...
   
   新功能描述英文
   修复描述英文
   重构描述英文英文
   ```
4. **Tag 提交**：尝试 `git tag v{x.y.z} && git push origin v{x.y.z}`；如果认证失败，提供命令让用户手动 push

## Common Commands

```bash
go test ./...
go build -o knot cmd/knot/main.go
go mod tidy
```
