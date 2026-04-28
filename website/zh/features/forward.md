# 端口转发

`knot forward` 管理本地、远程和动态端口转发规则。转发规则由 daemon 持有，可以使用已有 SSH 连接路径。

## 转发类型

| 类型 | 选项 | 用途 |
| --- | --- | --- |
| Local | `-L localPort:remoteAddr` | 在本机端口访问远程网络上的服务。 |
| Remote | `-R remotePort:localAddr` | 在远程端口暴露本地网络上的服务。 |
| Dynamic | `-D localPort` | 在本机启动动态 SOCKS5 转发。 |

## 添加规则

```sh
knot forward add [alias] [flags]
```

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-L, --local` | string | 空 | 本地转发，格式 `localPort:remoteAddr`。 |
| `-R, --remote` | string | 空 | 远程转发，格式 `remotePort:localAddr`。 |
| `-D, --dynamic` | int | `0` | 动态 SOCKS5 转发的本地端口。 |
| `-t, --temp` | bool | `false` | 创建临时规则，不保存到配置。 |

示例：

```sh
knot forward add web-prod -L 8080:127.0.0.1:80
knot forward add web-prod -D 1080 --temp
```

不提供转发选项时，Knot 会进入交互式创建流程。

## 列出规则

```sh
knot forward list [alias]
knot forward ls web-prod
```

输出包含 alias、类型、端口、目标地址、是否临时、状态和错误信息。使用 `--json` 可以得到结构化输出。

## 启用、禁用和删除

```sh
knot forward enable [alias] [type:port]
knot forward disable [alias] [type:port]
knot forward remove [alias] [type:port]
```

`type:port` 形如 `L:8080`、`R:9000` 或 `D:1080`。省略时，Knot 会交互式选择规则。

`knot forward remove` 的别名是 `knot forward rm`。

示例：

```sh
knot forward enable web-prod L:8080
knot forward disable web-prod L:8080
knot forward rm web-prod
```
