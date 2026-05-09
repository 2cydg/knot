# 配置同步

`knot sync` 用加密远端归档同步服务器、proxy 和托管密钥。它和 `knot export` / `knot import` 是两套语义：sync 适合日常多设备共享，import/export 仍然是完整本地备份和迁移流程。

当前支持 WebDAV 和 S3 兼容 provider。

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
knot sync provider add s3
knot sync provider add s3 home
```

这些形式都可以进入交互式流程。只输入 `provider add` 时，Knot 会先询问 provider 类型。支持的类型是 `webdav` 和 `s3`。输入 provider 类型后会从 alias 开始询问。alias 也输入后，会从对应 provider 参数开始询问。添加的第一个 provider 会自动设为默认，因此可以直接使用 `knot sync push` 和 `knot sync pull`，不必再输入 provider alias。

WebDAV 脚本中可以直接传参数：

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

URL 按目录处理时，Knot 会使用 `config.toml.enc` 作为远端文件名。上传前如果 WebDAV 目录不存在，并且服务器支持 `MKCOL`，Knot 会尝试创建目录。

示例：

| 输入 URL | 远端对象 |
| --- | --- |
| `https://dav.example.com/knot/config.toml.enc` | 原始文件路径 |
| `https://dav.example.com/knot/` | `https://dav.example.com/knot/config.toml.enc` |
| `https://dav.example.com/knot` | `https://dav.example.com/knot/config.toml.enc` |

AWS S3 示例：

```sh
knot sync provider add s3 home \
  --bucket my-bucket \
  --key knot/config.toml.enc \
  --region us-east-1 \
  --access-key-id "$S3_ACCESS_KEY_ID" \
  --secret-access-key "$S3_SECRET_ACCESS_KEY"
```

MinIO 等 S3-compatible 服务示例：

```sh
knot sync provider add s3 minio \
  --endpoint https://minio.example.com \
  --bucket knot \
  --key config.toml.enc \
  --region us-east-1 \
  --access-key-id minioadmin \
  --secret-access-key "$MINIO_SECRET_ACCESS_KEY" \
  --path-style
```

Cloudflare R2 这类服务需要显式 endpoint 和对应签名 region：

```sh
knot sync provider add s3 r2 \
  --endpoint https://<account-id>.r2.cloudflarestorage.com \
  --bucket knot \
  --region auto \
  --access-key-id "$R2_ACCESS_KEY_ID" \
  --secret-access-key "$R2_SECRET_ACCESS_KEY"
```

| 选项 | 说明 |
| --- | --- |
| `--bucket` | S3 bucket，必填。 |
| `--key` | S3 object key，默认 `config.toml.enc`。 |
| `--region` | S3 签名 region，必填。`auto` 只允许配合显式 endpoint。 |
| `--endpoint` | 可选的 S3-compatible endpoint URL；为空时使用 AWS S3 默认 endpoint。 |
| `--access-key-id` | S3 access key ID，会加密保存在本机配置中。 |
| `--secret-access-key` | S3 secret access key，会加密保存在本机配置中。 |
| `--session-token` | 可选 S3 session token，会加密保存在本机配置中。编辑时传 `-` 可清空。 |
| `--path-style` | 使用 path-style URL。AWS S3 通常不需要开启，除非 endpoint 明确要求。 |

默认情况下，Knot 使用 virtual-hosted-style S3 URL，也就是 bucket 放在主机名中：`https://bucket.s3.region.amazonaws.com/key`。这是 AWS S3 推荐的形式，支持 bucket host name 的 S3-compatible 服务也优先使用这种形式。

只有当你的 S3-compatible 服务要求把 bucket 放在路径中时才需要开启 `--path-style`，例如 `https://minio.example.com/bucket/key`。MinIO、本地测试服务，以及没有 wildcard DNS 或 bucket 专属 TLS 主机名的部署经常需要这种形式。这个选项只影响 URL 构造；请求签名仍然使用配置的 `--region`、凭据和 endpoint。

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
| `provider list` | 以表格列出 provider。WebDAV 显示 URL，S3 显示 `s3://bucket/key`。别名：`provider ls`。 |
| `provider show <alias>` | 查看单个 provider，不输出敏感信息。S3 凭据只显示 `has_*` 布尔值。 |
| `provider edit <alias>` | 编辑 provider。只输入 alias 时进入交互式。WebDAV 和 S3 的 edit flags 与 add flags 对应。 |
| `provider remove <alias>` | 删除 provider。别名：`rm`、`delete`。 |
| `provider set-default <alias>` | 将默认同步 provider 写入 `settings.default_sync_provider`。 |
| `provider clear-default` | 清空默认同步 provider。 |

也可以用全局配置设置默认 provider：

```sh
knot config set default_sync_provider home
```

## 同步密码

同步归档在上传前会用同步密码加密。同步密码和 WebDAV 密码、S3 凭据都是两件事。Provider 凭据只保存在本机，不会写入同步归档。

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
