# Proxy 代理

`knot proxy` 管理可复用的 HTTP 和 SOCKS5 proxy。服务器配置通过 `--proxy` 引用 proxy 别名，避免在每台服务器上重复填写代理地址和认证信息。

## 命令概览

```sh
knot proxy [command]
```

| 命令 | 说明 |
| --- | --- |
| `knot proxy list` | 列出已配置 proxy。别名：`knot proxy ls`。 |
| `knot proxy add` | 添加或覆盖托管 proxy。 |
| `knot proxy edit` | 交互式编辑 proxy。 |
| `knot proxy remove` | 删除 proxy。别名：`knot proxy rm`。 |

## 添加 Proxy

```sh
knot proxy add [alias] [flags]
```

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--type` | string | 空 | 代理类型：`socks5` 或 `http`。 |
| `--host` | string | 空 | 代理主机。 |
| `--port` | int | `0` | 代理端口。 |
| `--user` | string | 空 | 代理用户名。 |
| `--password` | string | 空 | 代理密码。 |

提供 `--type`、`--host` 和 `--port` 时，命令以非交互模式运行：

```sh
knot proxy add corp --type socks5 --host 127.0.0.1 --port 1080
```

## 在服务器中引用 Proxy

```sh
knot add app --host 10.0.1.20 --user deploy --proxy corp
knot edit app --proxy corp
```

清除服务器 proxy 时，使用 `knot edit` 显式传入空值。

## 删除 Proxy

```sh
knot proxy remove corp
knot proxy rm corp
```

如果已有服务器引用该 proxy，Knot 会询问后再清除这些引用。
