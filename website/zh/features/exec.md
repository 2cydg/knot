# 远程执行

`knot exec` 用于在远程服务器执行非交互命令。它适合脚本、CI 和 AI agent，因为命令的退出码会被保留，输出也可以用 JSON 格式消费。

## `knot exec`

```sh
knot exec [alias] [command...] [flags]
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `[alias]` | 是 | 服务器别名。 |
| `[command...]` | 是 | 远程命令及参数。Knot 会用空格拼接。 |

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-t, --timeout` | int | `60` | 命令超时时间，单位秒。使用 `0` 表示不限制。 |
| `--json` | bool | `false` | 输出结构化 JSON。 |

示例：

```sh
knot exec web-prod "uptime"
knot exec web-prod "systemctl status nginx" --json
knot exec web-prod "tail -n 100 /var/log/nginx/error.log" --timeout 10
```

## 自动化建议

- 需要机器读取时使用 `--json`。
- 长任务明确设置 `--timeout 0` 或一个更大的超时时间。
- 复杂 shell 逻辑建议作为一个带引号的远程命令传入，例如 `knot exec web "sh -lc 'cd /srv/app && git status'"`。
