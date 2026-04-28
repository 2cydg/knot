# Daemon 与状态

Knot 的后台 daemon 负责持久 SSH 连接、远程执行、SFTP、端口转发、状态查询和连接清理。CLI 在需要时可以自动启动 daemon。

## Daemon 命令

```sh
knot daemon [command]
```

| 命令 | 说明 |
| --- | --- |
| `knot daemon start` | 启动 daemon。隐藏快捷写法：`knot start`。 |
| `knot daemon stop` | 停止 daemon。隐藏快捷写法：`knot stop`。 |
| `knot daemon restart` | 重启 daemon。隐藏快捷写法：`knot restart`。 |
| `knot daemon clear` | 断开 daemon 当前持有的所有活跃 SSH 连接。隐藏快捷写法：`knot clear`。 |

`knot daemon start` 支持：

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-f, --foreground` | bool | `false` | 以前台方式运行 daemon。 |

示例：

```sh
knot daemon start
knot daemon start --foreground
knot daemon clear
```

## 状态

```sh
knot status
knot status --json
```

`knot status` 显示 daemon 状态和连接池统计。脚本读取时使用 `--json`。

## 日志

```sh
knot logs [flags]
```

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-t, --tail` | int | `100` | 从日志末尾显示的行数。 |
| `-f, --follow` | bool | `false` | 持续跟随日志输出。 |

示例：

```sh
knot logs
knot logs --tail=50
knot logs --tail=20 -f
```
