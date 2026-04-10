# Knot

Knot is a minimalist, high-speed, and secure SSH/SFTP command-line management tool designed for developers and AI Agents.

## Core Philosophy
- **Minimalist Interaction**: Pure CLI with no TUI, making it friendly for automation and AI integration.
- **Connection Multiplexing**: A background daemon manages physical connections, allowing for instant new sessions.
- **Native Security**: Deep integration with OS-level secure storage (e.g., Windows DPAPI, Linux Machine-ID + Salt) to keep credentials safe.

## Key Features
- **C/S Architecture**: Frontend CLI communicates with a backend Daemon via Unix Domain Socket (UDS).
- **Fast Response**: Drastically reduces connection overhead by reusing existing SSH clients.
- **Secure Storage**: Sensitive information is encrypted and stored in TOML configurations with an `ENC:` prefix.
- **Interactive SFTP REPL**: A powerful environment for file operations with completion and history support.
- **Directory Following**: Syncs SSH and SFTP working directories via OSC 7.

## Architecture
- **Language**: Go (Golang)
- **Protocols**: `golang.org/x/crypto/ssh`, `github.com/pkg/sftp`
- **Communication**: Local UDS for high-performance CLI-to-Daemon interaction.

## Getting Started

### Prerequisites
- Go 1.21 or higher (for building from source)
- Linux (Windows and macOS support in progress)

### Installation
```bash
go build -o knot cmd/knot/main.go
```

### Usage
```bash
# Add a new server configuration
./knot add [alias]

# List all servers
./knot list

# Connect via SSH
./knot ssh [alias]

# Enter SFTP REPL
./knot sftp [alias]

# Export/Import configurations
./knot export
./knot import [file]
```

## License
MIT License. See [LICENSE](LICENSE) for details.
