# SSH 密钥

`knot key` 管理由 Knot 保存的 SSH 私钥。服务器配置通过 `--key` 引用托管密钥别名。

## 命令概览

```sh
knot key [command]
```

| 命令 | 说明 |
| --- | --- |
| `knot key list` | 列出托管密钥。别名：`knot key ls`。 |
| `knot key add [alias]` | 添加或覆盖托管密钥。 |
| `knot key edit [alias]` | 替换已有密钥内容。 |
| `knot key remove [alias]` | 删除托管密钥。别名：`knot key rm`。 |

## 添加密钥

```sh
knot key add [alias] [flags]
```

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--file` | string | 空 | 私钥文件路径。 |
| `--content` | string | 空 | 私钥内容。 |
| `--passphrase` | string | 空 | 加密私钥的口令。可能出现在进程列表中。 |

示例：

```sh
knot key add deploy --file ~/.ssh/id_ed25519
knot key add deploy
```

涉及 passphrase 时，交互模式通常更安全。

## 在服务器中使用密钥

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy
knot edit web-prod --key deploy
```

设置密钥别名时，如果需要，Knot 会把认证方式切到 key。

## 删除密钥

```sh
knot key remove deploy
knot key rm deploy
```

如果服务器正在引用该密钥，Knot 会询问后再清除引用。
