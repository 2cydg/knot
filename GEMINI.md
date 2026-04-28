# Knot Project Instructions for Gemini CLI

## Project Philosophy

Knot is a minimalist SSH/SFTP connection manager designed for native terminals, automated scripts, and AI agents. It prioritizes a seamless CLI-first workflow over complex TUIs, allowing users to stay within their preferred terminal environment (e.g., Windows Terminal, iTerm2, Kitty).

Key tenets include:
- **C/S Architecture**: A lightweight CLI (frontend) communicates with a long-running background Daemon (backend) via Unix Domain Sockets (UDS).
- **Connection Multiplexing**: Physical SSH connections are kept alive by the daemon and reused across multiple shells, commands, and file transfers to eliminate handshake overhead.
- **Security First**: Sensitive credentials (passwords, private keys) are never stored in plaintext. They are encrypted using platform-native facilities (Windows DPAPI, macOS Keychain, Linux Secret Service/AES-GCM).
- **Minimalist & Scriptable**: Focuses on clean, composable commands with structured `--json` output, making it ideal for integration into CI/CD pipelines and AI-driven development.
- **Seamless Connectivity**: Simplifies complex network paths involving jump host chains and proxies (SOCKS5/HTTP).

## Core Logic

- **CLI Layer (`cmd/knot/commands`)**: Built with Cobra, it handles user input, flag parsing, and shell completion. It serves as the primary interface, often auto-starting the daemon and translating user commands into protocol requests.
- **Daemon Layer (`pkg/daemon`)**: The engine of Knot. it manages the lifecycle of SSH sessions, SFTP operations, and port forwarding rules. It listens on a local UDS for CLI requests.
- **Protocol (`internal/protocol`)**: A custom, low-latency binary protocol for IPC. It uses an 8-byte header (Magic "KN", Version, Type, Subtype/Reserved, and Length) to frame JSON payloads.
- **SSH Pooling (`pkg/sshpool`)**: Manages a pool of reusable `ssh.Client` instances. It handles connection parameters, keepalives, idle cleanups, and reference counting to ensure efficient resource usage.
- **Configuration & Security (`pkg/config` & `pkg/crypto`)**: Handles encrypted TOML configuration. Secrets are prefixed with `ENC:` and managed by platform-specific crypto providers (DPAPI on Windows, Keychain on macOS, AES-256-GCM on Linux).
- **SFTP & File Transfer (`pkg/sftp`)**: Implements an interactive SFTP REPL and high-speed file transfer logic. The `knot cp` command uses a Docker-like `alias:/path` syntax for intuitive local-remote transfers.

## Engineering Standards

- **Commit Messages**: ALWAYS use English for git commit messages. Follow Conventional Commits (e.g., `feat: ...`, `fix: ...`).
- **Idiomatic Go**: Maintain clean, idiomatic Go code following existing package boundaries. Avoid unrelated refactors.
- **Testing**: Add focused unit or regression tests for any core logic changes. Always run `go test ./...` before concluding a task.
- **Validation**: Ensure the project builds successfully using `go build -o knot cmd/knot/main.go`.
- **Implementation Alignment**: Ensure all new features align with the minimalist and secure architectural vision.
- **Dependency Management**: Avoid adding unnecessary dependencies to keep the binary size minimal. When a dependency is required, prioritize official Go standard library packages or sub-repositories (`golang.org/x/...`) over third-party libraries.

## Key Commands

- `go mod tidy`: Update and clean up dependencies.
- `go test ./...`: Execute all unit tests.
- `go build -o knot cmd/knot/main.go`: Build the main binary.
- `knot status`: Check the daemon health and connection pool state.
