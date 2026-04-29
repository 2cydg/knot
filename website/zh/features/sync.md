# 配置同步

`knot sync` 用加密远端归档同步服务器、proxy 和托管密钥。它和 `knot export` / `knot import` 是两套语义：sync 适合日常多设备共享，import/export 仍然是完整本地备份和迁移流程。

当前只支持 WebDAV provider。

## 同步范围

同步归档包含：

| 配置段 | 是否同步 |
| --- | --- |
| `servers` | 是 |
| `proxies` | 是 |
| `keys` | 是 |
| `settings` | 否 |
| `sync_providers` | 否 |
| daemon 状态、日志、known hosts | 否 |

这样每台机器自己的偏好和 provider 凭据会保留在本机。

## 配置 Provider

```sh
knot sync provider add
knot sync provider add webdav
knot sync provider add webdav home
```

这些形式都可以进入交互式流程。只输入 `provider add` 时，Knot 会先询问 provider 类型。当前类型只有 `webdav`。输入 `webdav` 后会从 alias 开始询问。alias 也输入后，会从 WebDAV 参数开始询问。

脚本中可以直接传 WebDAV 参数：

```sh
knot sync provider add webdav home \
  --url https://dav.example.com/knot/ \
  --user alice \
  --password "$WEBDAV_PASSWORD"
```

| 选项 | 说明 |
| --- | --- |
| `--url` | WebDAV URL。看起来像文件路径的 URL 会原样使用，其他 URL 按目录处理。 |
| `--user` | WebDAV 用户名。 |
| `--password` | WebDAV 密码，会加密保存在本机配置中。 |

URL 按目录处理时，Knot 会使用 `config-sync.toml.enc` 作为远端文件名。上传前如果 WebDAV 目录不存在，并且服务器支持 `MKCOL`，Knot 会尝试创建目录。

示例：

| 输入 URL | 远端对象 |
| --- | --- |
| `https://dav.example.com/knot/config-sync.toml.enc` | 原始文件路径 |
| `https://dav.example.com/knot/` | `https://dav.example.com/knot/config-sync.toml.enc` |
| `https://dav.example.com/knot` | `https://dav.example.com/knot/config-sync.toml.enc` |

## Provider 命令

```sh
knot sync provider list
knot sync provider ls
knot sync provider show home
knot sync provider edit home
knot sync provider remove home
knot sync provider rm home
knot sync provider set-default home
knot sync provider clear-default
```

| 命令 | 说明 |
| --- | --- |
| `provider list` | 以表格列出 provider。别名：`provider ls`。 |
| `provider show <alias>` | 查看单个 provider，不输出敏感信息。 |
| `provider edit <alias>` | 编辑 provider。只输入 alias 时进入交互式。 |
| `provider remove <alias>` | 删除 provider。别名：`rm`、`delete`。 |
| `provider set-default <alias>` | 将默认同步 provider 写入 `settings.default_sync_provider`。 |
| `provider clear-default` | 清空默认同步 provider。 |

也可以用全局配置设置默认 provider：

```sh
knot config set default_sync_provider home
```

## 同步密码

同步归档在上传前会用同步密码加密。同步密码和 WebDAV 密码是两件事。

```sh
knot sync password set
knot sync password set --password-stdin
knot sync password status
knot sync password clear
```

如果本机没有保存同步密码，`push` 和 `pull` 会交互式询问。脚本中使用 `--password-stdin`。

## Push

```sh
knot sync push
knot sync push home
knot sync push --provider home
```

`push` 会导出本机的 `servers`、`proxies` 和 `keys`，用同步密码加密，然后上传到选中的 provider。交互式终端中，除非传入 `--force`，否则会在覆盖远端归档前确认。

| 选项 | 说明 |
| --- | --- |
| `--provider <alias>` | 指定 provider，优先级高于位置参数和默认 provider。 |
| `--password-stdin` | 从 stdin 读取同步密码。 |
| `--no-save-password` | 本次不保存同步密码。 |
| `--force` | 跳过覆盖确认。 |

## Pull

```sh
knot sync pull home --strategy local-first
knot sync pull --provider home --strategy remote-first
knot sync pull home --strategy overwrite --dry-run
```

`pull` 会下载加密归档、解密，然后把远端的 `servers`、`proxies` 和 `keys` 合并到本机配置。本机 `settings` 和 `sync_providers` 始终保留。

| 选项 | 说明 |
| --- | --- |
| `--provider <alias>` | 指定 provider。 |
| `--strategy <name>` | 合并策略：`local-first`、`remote-first` 或 `overwrite`。 |
| `--password-stdin` | 从 stdin 读取同步密码。 |
| `--dry-run` | 只显示合并摘要，不写入本机配置。 |
| `--force` | 在适用场景中跳过确认。 |

非交互模式下需要显式传入 `--strategy`。

## 合并策略

| 策略 | 行为 |
| --- | --- |
| `local-first` | 按 alias 匹配。冲突时本地优先，只添加远端独有项。 |
| `remote-first` | 按 alias 匹配。冲突时远端优先，本地独有项会保留。 |
| `overwrite` | 用远端归档替换本机 `servers`、`proxies` 和 `keys`。本机 `settings` 和 `sync_providers` 保留。 |

合并时 Knot 会重映射内部 ID，确保 server 对 key、proxy 和 jump host 的引用继续指向最终保留的对象。

