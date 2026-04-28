# 服务器与全局配置

Knot 的配置包含服务器、proxy、托管密钥和全局设置。敏感值会以 `ENC:` 前缀保存，底层使用平台安全能力或机器绑定的 Linux fallback。

## 服务器命令

| 命令 | 说明 |
| --- | --- |
| `knot add [alias]` | 添加或覆盖服务器配置。 |
| `knot edit [alias]` | 编辑已有服务器配置。 |
| `knot list [pattern]` | 列出服务器。别名：`knot ls`。 |
| `knot remove [alias]` | 删除服务器。别名：`knot rm`、`knot delete`。 |

常用服务器字段：

| 字段 | 来源选项 | 说明 |
| --- | --- | --- |
| Alias | `[alias]` 或 `--alias` | 服务器别名，也是大多数命令使用的入口。 |
| Host | `--host` | 主机名或 IP。 |
| Port | `--port` | SSH 端口，默认 `22`。 |
| User | `--user` | SSH 用户名。 |
| Auth | `--auth-method`、`--password`、`--key` | 密码、托管私钥或 agent 认证。 |
| Jump hosts | `--jump-host` | 逗号分隔的跳板机别名链。 |
| Proxy | `--proxy` | 托管 proxy 别名。 |
| Tags | `--tags` | 逗号分隔标签，用于筛选和组织。 |

## 全局配置

```sh
knot config [command]
```

| 命令 | 说明 |
| --- | --- |
| `knot config init` | 初始化配置，或将全局设置重置为默认值。已有 servers、proxies 和 keys 会保留。 |
| `knot config list` | 列出全局设置。别名：`knot config ls`。 |
| `knot config get [path]` | 输出完整脱敏配置，或输出某个脱敏路径。 |
| `knot config set [key] [value]` | 设置全局配置。 |

`knot config get` 中不带点号的路径会优先解析为 `settings` 下的 key：

```sh
knot config get
knot config get log_level
knot config get servers.web-prod
```

## 可设置项

| Key | 值类型 | 说明 |
| --- | --- | --- |
| `forward_agent` | bool | 是否转发 SSH agent。 |
| `clear_screen_on_connect` | bool | 连接后是否清屏。 |
| `idle_timeout` | Go duration | daemon 中空闲连接的超时时间。 |
| `keepalive_interval` | Go duration | SSH keepalive 间隔。 |
| `log_level` | `debug`、`info`、`warn`、`error` | 日志级别。 |

示例：

```sh
knot config set forward_agent true
knot config set idle_timeout 30m
knot config set log_level error
```

配置变更应用到新连接。

## 导入与导出

```sh
knot export [path]
knot import [path]
```

`knot export` 会把配置导出成密码加密归档，默认输出 `config.toml.enc`。

`knot import` 会导入密码加密归档，并让你选择合并策略：

```sh
knot export backup.enc
knot import backup.enc
```
