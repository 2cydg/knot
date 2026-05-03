# SSH 连接

SSH 是 Knot 的核心入口。你先用 `knot add` 保存服务器别名，再用 `knot ssh [alias]` 打开交互式 SSH 会话。后台 daemon 会维护可复用连接，后续 `exec`、`sftp`、`cp` 和转发操作可以复用同一条连接路径。

## `knot ssh`

```sh
knot ssh [alias]
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `[alias]` | 是 | 要连接的服务器别名。 |

示例：

```sh
knot ssh web-prod
```

### 快捷写法

当根命令收到一个未知的第一个参数时，会把它当作服务器别名并重写为 `knot ssh [alias]`：

```sh
knot web-prod
```

等价于：

```sh
knot ssh web-prod
```

为了避免歧义，快捷别名不能和内置命令同名，也不能包含空白、路径分隔符或常见 shell 元字符。

### 命令广播

交互式 SSH 会话可以加入 daemon 本地维护的广播组。同一组内的 active 成员会互相转发键盘输入：

```sh
knot ssh web-1 --broadcast cloud
knot web-2 --broadcast cloud
```

可以用 `knot broadcast list` 和 `knot broadcast show cloud` 查看广播组。`knot broadcast pause`、`resume`、`leave` 和 `disband` 可以管理已有会话，不会关闭 SSH 连接。

会话内 escape 控制默认关闭。可以用 `--escape` 为单次连接开启默认前缀，用 `--escape %` 指定单字符前缀，或用 `--escape none` 强制关闭：

```sh
knot ssh web-1 --broadcast cloud --escape
knot ssh web-1 --broadcast cloud --escape %
knot ssh web-1 --broadcast cloud --escape none
```

全局配置 `broadcast_escape_enable` 和 `broadcast_escape_char` 控制带 `--broadcast` 启动的 SSH 会话默认行为。命令行里的 `--escape` 优先级始终高于全局配置。普通 `knot ssh [alias]` 会话不会因为全局配置自动启用 broadcast escape。开启 escape 后，Knot 会在连接后打印可用的本地快捷键。

## 添加服务器

```sh
knot add [alias] [flags]
```

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-H, --host` | string | 空 | 服务器主机名或 IP 地址。 |
| `-P, --port` | int | `22` | SSH 端口。 |
| `-u, --user` | string | 空 | SSH 用户名。 |
| `-p, --password` | string | 空 | SSH 密码。能交互输入时更建议交互输入。 |
| `-k, --key` | string | 空 | 使用的托管密钥别名。 |
| `--auth-method` | string | 空 | 认证方式：`password`、`key` 或 `agent`。 |
| `--known-hosts` | string | 空 | 该服务器使用的 known_hosts 文件路径。 |
| `-J, --jump-host` | string | 空 | 逗号分隔的跳板机别名链。 |
| `--proxy` | string | 空 | 托管 proxy 别名。 |
| `-t, --tags` | string | 空 | 逗号分隔的服务器标签。 |

示例：

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
knot add db --host 10.0.0.5 --user root --jump-host bastion
```

提供 host 和 user 时，命令以非交互模式运行；否则进入引导式配置。

## 编辑服务器

```sh
knot edit [alias] [flags]
```

常用选项与 `knot add` 基本一致，并额外支持：

| 选项 | 说明 |
| --- | --- |
| `--alias` | 重命名服务器别名。 |
| `-J, --jump-host` | 设置跳板机链；显式传空值可清除跳板机。 |
| `--proxy` | 设置托管 proxy；显式传空值可清除 proxy。 |
| `-t, --tags` | 设置标签；显式传空值可清除标签。 |

示例：

```sh
knot edit web-prod --host 1.2.3.5
knot edit web-prod --alias web-blue --tags prod,blue
```

未提供编辑选项时，Knot 会打开交互式编辑流程。

## 列出与删除服务器

```sh
knot list [pattern]
knot remove [alias]
```

`knot list` 的别名是 `knot ls`，可以按别名、用户、主机或标签做大小写不敏感过滤。

`knot remove` 的别名是 `knot rm` 和 `knot delete`。

```sh
knot list prod
knot rm old-host
```

## 跳板机与 Proxy

服务器配置可以同时描述跳板机和 proxy 路径：

```sh
knot add app --host 10.0.1.20 --user deploy --jump-host bastion --proxy corp
```

- `--jump-host` 引用已经存在的服务器别名，多个跳板机用逗号分隔。
- `--proxy` 引用 [Proxy 代理](/zh/features/proxy) 中维护的托管 proxy。
