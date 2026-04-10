# Knot Project Instructions for Gemini CLI

## Project Principles
- **C/S Architecture**: The CLI (frontend) communicates with a background Daemon (backend) via Unix Domain Socket (UDS).
- **No TUI**: Maintain a clean, command-line interface suitable for AI Agents and scripts.
- **Connection Multiplexing**: Reuse SSH physical connections.
- **Security**: Never store passwords in plaintext. Use platform-specific providers.

## Engineering Standards
- **Commit Messages**: ALWAYS use English for git commit messages. This is a mandatory requirement for the entire development cycle.
- **Testing**: Always run `go test ./...` after any core logic changes.
- **Validation**: Ensure `knot build` or `go build -o knot cmd/knot/main.go` succeeds.
- **Implementation Alignment**: Ensure features match the overall architectural vision of being a minimalist and secure SSH/SFTP manager.

## Key Commands
- `go mod tidy`: Update dependencies.
- `go test ./...`: Run all unit tests.
- `go build -o knot cmd/knot/main.go`: Build the binary.
