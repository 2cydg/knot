# Knot

Knot 是一款专为开发者与 AI Agent 设计的极简、高速且安全的 SSH/SFTP 命令行管理工具。

## 核心理念
- **极简交互**：拥抱纯 CLI，无 TUI（终端用户界面），对自动化脚本和 AI 集成极其友好。
- **连接复用**：通过后台守护进程 (Daemon) 维护 SSH 物理长连接，实现秒级开启新会话。
- **原生安全**：凭据存储与操作系统深度集成（如 Windows DPAPI, Linux Machine-ID + Salt），杜绝明文密码。

## 主要特性
- **C/S 架构**：前端 CLI 与后端 Daemon 通过 Unix Domain Socket (UDS) 进行高效通信。
- **极速响应**：通过连接池技术，大幅降低新会话的建立开销。
- **安全存储**：敏感信息加密后以 `ENC:` 前缀存储于 TOML 配置文件中。
- **交互式 SFTP REPL**：提供功能强大的交互环境，支持命令补全与历史记录。
- **目录跟随**：通过监听 PTY 数据流（OSC 7），支持 SSH 与 SFTP 工作目录实时同步。

## 技术架构
- **语言**：Go (Golang)
- **协议**：`golang.org/x/crypto/ssh`, `github.com/pkg/sftp`
- **通信**：本地 UDS 确保高性能与本地安全性。

## 快速入门

### 前置条件
- Go 1.21 或更高版本（用于从源码构建）
- Linux (Windows 与 macOS 支持正在开发中)

### 安装
```bash
go build -o knot cmd/knot/main.go
```

### 常用命令
```bash
# 添加新的服务器配置
./knot add [alias]

# 列出所有服务器
./knot list

# 通过 SSH 连接
./knot ssh [alias]

# 进入 SFTP 交互环境
./knot sftp [alias]

# 导出/导入配置
./knot export
./knot import [file]
```

## 许可证
MIT License. 详见 [LICENSE](LICENSE) 文件。
