# SFTP 与文件复制

Knot 提供两类文件操作：交互式 `knot sftp` shell，以及面向脚本的批处理命令。`knot cp` 使用 Docker 风格的 `alias:/path` 远程路径，适合本地和远程之间复制文件或目录。

## 交互式 SFTP

```sh
knot sftp [alias] [remote_path]
knot sftp [alias] --follow
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `[alias]` | 是 | 服务器别名。 |
| `[remote_path]` | 否 | 初始远程目录。 |

| 选项 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--follow` | bool | `false` | 跟随同一 alias 下活动 `knot ssh` session 的当前目录。不能和 `[remote_path]` 同时使用。 |

示例：

```sh
knot sftp web-prod
knot sftp web-prod /var/www
knot sftp web-prod --follow
```

交互式 shell 支持命令补全和路径补全。远程 `~` 会解析到当前连接用户的 home 目录。如果配置了 `settings.default_sftp_local_path`，REPL 中的相对本地路径会基于该目录解析，本地路径 Tab 补全也会从该目录开始。

## 跟随 SSH 目录

`knot sftp web-prod --follow` 会打开一个交互式 SFTP shell，并跟随已有的 `knot ssh web-prod` session。如果当前只有一个活动 SSH session，Knot 会自动跟随它；如果有多个，Knot 会列出带序号的 session、启动时间和已知当前目录，并提示用户输入 `No.` 列中的序号选择。

被跟随的 SSH session 上报新目录后，SFTP shell 会尝试切换到同一路径，并输出简短的 `[follow]` 提示。如果该目录无法通过 SFTP 打开，SFTP shell 保持当前目录不变并继续等待下一次更新。被跟随的 SSH session 关闭后，SFTP shell 保持打开，停留在最后的目录并停止跟随。

目录跟随依赖 OSC 7 current-directory escape sequence。`knot sftp --follow` 绑定到活动 SSH session 时，Knot 会向被跟随的 session 注入一段临时 OSC 7 hook，适用于 Bash 和 Zsh。其他 shell 只有在自己已经输出 OSC 7 时才会生效；如果没有 OSC 7，Knot 无法跟踪目录变化。

这段 hook 会作为键盘输入发送到被跟随的 SSH session，并以 Enter 结束。请只在被跟随的 session 停在普通 shell prompt 时使用 `--follow`。不要在 Vim、less、top、数据库 shell、语言 REPL 或其他全屏/交互程序处于前台时启动 follow，否则 hook 文本可能会被这些前台程序处理。这个 hook 只在当前 session 内临时存在，不会修改远端 `.bashrc`、`.zshrc` 或其他 shell 配置文件。

## SFTP shell 命令

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `help` | `help` 或 `?` | 显示帮助。 |
| `clear` | `clear` | 清空当前终端输出。 |
| `exit` | `exit`、`quit` 或 `bye` | 退出 SFTP shell。 |
| `ls` | `ls [path]` | 列出远程目录内容。 |
| `pwd` | `pwd` | 输出当前远程目录。 |
| `cd` | `cd <path>` | 切换远程目录。`cd ~` 会跳到远端 home 目录。 |
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
sftp:/var/www> clear
sftp:/var/www> cd ~
sftp:/var/www> put ./dist/app.tar.gz /tmp/app.tar.gz
sftp:/var/www> get release.tar.gz ~/Downloads/
```

## 默认本地目录

可以为交互式 SFTP shell 配置默认本地目录：

```sh
knot config set default_sftp_local_path ~/Downloads
```

配置 `default_sftp_local_path` 后：

- `get release.tar.gz` 会下载到 `<default_sftp_local_path>/release.tar.gz`。
- `get -r logs` 会下载到 `<default_sftp_local_path>/logs`。
- `put app.tar.gz /tmp/` 会从 `<default_sftp_local_path>/app.tar.gz` 读取本地文件。
- `mget` 和 `mput` 在省略相对本地路径时会使用同一个本地基准目录。
- 绝对本地路径、`~/...` 和 Windows 盘符路径仍按显式路径处理，不会再拼到默认目录下。

## 远程路径规则

- 远程绝对路径，例如 `/var/www`，按原样使用。
- 远程相对路径会基于当前 SFTP 目录解析。
- `~` 表示远端用户的 home 目录。
- `~/logs` 表示远端 home 目录下的 `logs`。

## 上传规则

这些规则同时适用于交互式 `put` 和批量上传 `knot cp`。

### 上传文件

假设当前远端目录是 `/cwd`，本地源文件是 `file.txt`。

| 命令 | 结果 |
| --- | --- |
| `put file.txt` | 上传到 `/cwd/file.txt`。 |
| `put file.txt remote.txt` | 如果 `remote.txt` 不是目录，则上传为 `/cwd/remote.txt`。 |
| `put file.txt remote/` | 把 `remote/` 当作目录，上传到 `/cwd/remote/file.txt`。缺失目录会自动创建。 |
| `put file.txt existing_dir` | 如果 `existing_dir` 是远端目录，则上传到 `/cwd/existing_dir/file.txt`。 |
| `put file.txt/` | 返回清晰错误，因为本地源是文件而不是目录。 |

### 上传目录

假设当前远端目录是 `/cwd`，本地源目录是 `dist`。

| 命令 | 结果 |
| --- | --- |
| `put -r dist` | 把目录本身上传到 `/cwd/dist`。 |
| `put -r dist/` | 与 `put -r dist` 等价，尾部 `/` 不改变语义。 |
| `put -r dist/.` | 把 `dist` 的内容上传到 `/cwd`，不会额外创建 `/cwd/dist`。 |
| `put -r dist release` | 如果 `release` 不存在，则把目录上传为 `/cwd/release`；如果 `release` 是目录，则上传到 `/cwd/release/dist`。 |
| `put -r dist release/` | 把 `release/` 当作容器目录，上传到 `/cwd/release/dist`。缺失目录会自动创建。 |
| `put -r dist/. release` | 把 `dist` 的内容上传到 `/cwd/release`。缺失目录会自动创建。 |
| `put -r dist existing_file` | 返回清晰错误，因为目标已经存在且是文件。 |

## 下载规则

假设当前远端目录是 `/cwd`，远端源是 `release.tar.gz` 或 `logs`。

| 命令 | 结果 |
| --- | --- |
| `get release.tar.gz` | 下载到 `./release.tar.gz`，如果配置了默认本地目录则下载到 `<default_sftp_local_path>/release.tar.gz`。 |
| `get release.tar.gz out.tar.gz` | 下载为 `out.tar.gz`。 |
| `get release.tar.gz target/` | 把 `target/` 当作目录，下载到 `target/release.tar.gz`。缺失目录会自动创建。 |
| `get -r logs` | 把目录本身下载到 `./logs`，如果配置了默认本地目录则下载到 `<default_sftp_local_path>/logs`。 |
| `get -r logs/` | 与 `get -r logs` 等价，尾部 `/` 不改变语义。 |
| `get -r logs/.` | 把 `logs` 的内容下载到本地目标目录，不额外创建一层 `logs`。 |
| `get -r logs archive` | 如果 `archive` 不存在，则把目录下载为 `archive`；如果 `archive` 已存在且是目录，则下载到 `archive/logs`。 |

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

对于 `knot cp`，`dist` 和 `dist/` 等价，都会复制目录本身。只有 `dist/.` 才表示复制目录内容而不包含最外层目录。
