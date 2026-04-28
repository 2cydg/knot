# SFTP 与文件复制

Knot 提供两类文件操作：交互式 `knot sftp` shell，以及面向脚本的批处理命令。`knot cp` 使用 Docker 风格的 `alias:/path` 远程路径，适合本地和远程之间复制文件或目录。

## 交互式 SFTP

```sh
knot sftp [alias] [remote_path]
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `[alias]` | 是 | 服务器别名。 |
| `[remote_path]` | 否 | 初始远程目录。 |

示例：

```sh
knot sftp web-prod
knot sftp web-prod /var/www
```

交互式 shell 支持命令补全和路径补全。

## SFTP shell 命令

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `help` | `help` 或 `?` | 显示帮助。 |
| `exit` | `exit`、`quit` 或 `bye` | 退出 SFTP shell。 |
| `ls` | `ls [path]` | 列出远程目录内容。 |
| `pwd` | `pwd` | 输出当前远程目录。 |
| `cd` | `cd <path>` | 切换远程目录。 |
| `get` | `get <remote_path> [local_path]` | 下载文件或目录。 |
| `put` | `put <local_path> [remote_path]` | 上传文件或目录。 |
| `mget` | `mget <remote_pattern> [local_dir]` | 按通配符批量下载。 |
| `mput` | `mput <local_pattern> [remote_dir]` | 按通配符批量上传。 |
| `rm` | `rm <path>` | 删除远程文件。 |
| `mkdir` | `mkdir <path>` | 创建远程目录。 |
| `rmdir` | `rmdir <path>` | 删除远程目录。 |

示例：

```text
sftp:/var/www> ls
sftp:/var/www> put ./dist/app.tar.gz /tmp/app.tar.gz
sftp:/var/www> get release.tar.gz ~/Downloads/
```

## 批处理 SFTP 命令

这些命令不进入交互式 shell，适合脚本：

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `knot sftp ls` | `knot sftp ls alias:/path` | 列出远程目录。 |
| `knot sftp stat` | `knot sftp stat alias:/path` | 显示远程文件或目录元数据。 |
| `knot sftp rm` | `knot sftp rm alias:/path` | 删除远程文件。 |
| `knot sftp mkdir` | `knot sftp mkdir alias:/path` | 创建远程目录，包括缺失的父目录。 |
| `knot sftp rmdir` | `knot sftp rmdir alias:/path` | 删除远程目录。 |
| `knot sftp mv` | `knot sftp mv alias:/old alias:/new` | 重命名远程文件或目录。源和目标必须在同一 alias 上。 |

示例：

```sh
knot sftp ls web-prod:/var/www
knot sftp stat web-prod:/var/www/index.html
knot sftp mv web-prod:/tmp/a web-prod:/tmp/b
```

## `knot cp`

```sh
knot cp [SRC] [DEST] [flags]
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `[SRC]` | 是 | 本地路径，或 `alias:/path` 形式的远程路径。 |
| `[DEST]` | 是 | 本地路径，或 `alias:/path` 形式的远程路径。不支持远程到远程复制。 |

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-r, --recursive` | bool | `true` | 递归复制目录。 |
| `-f, --force` | bool | `false` | 覆盖已有文件。 |

示例：

```sh
knot cp ./dist/. web-prod:/var/www/html/
knot cp web-prod:/var/log/nginx/access.log ./
```

源目录以 `/.` 结尾时复制目录内容，而不是目录本身。
