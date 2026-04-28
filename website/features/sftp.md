# SFTP and File Copy

Knot provides two styles of file operation: the interactive `knot sftp` shell and script-friendly batch commands. `knot cp` uses Docker-style `alias:/path` remote paths for copying between the local machine and one remote server.

## Interactive SFTP

```sh
knot sftp [alias] [remote_path]
```

| Argument | Required | Description |
| --- | --- | --- |
| `[alias]` | Yes | Server alias. |
| `[remote_path]` | No | Initial remote directory. |

Examples:

```sh
knot sftp web-prod
knot sftp web-prod /var/www
```

The interactive shell supports command and path completion.

## SFTP Shell Commands

| Command | Usage | Description |
| --- | --- | --- |
| `help` | `help` or `?` | Show help. |
| `exit` | `exit`, `quit`, or `bye` | Exit the SFTP shell. |
| `ls` | `ls [path]` | List remote directory contents. |
| `pwd` | `pwd` | Print the current remote directory. |
| `cd` | `cd <path>` | Change remote directory. |
| `get` | `get <remote_path> [local_path]` | Download a file or directory. |
| `put` | `put <local_path> [remote_path]` | Upload a file or directory. |
| `mget` | `mget <remote_pattern> [local_dir]` | Download files matching a wildcard. |
| `mput` | `mput <local_pattern> [remote_dir]` | Upload files matching a wildcard. |
| `rm` | `rm <path>` | Remove a remote file. |
| `mkdir` | `mkdir <path>` | Create a remote directory. |
| `rmdir` | `rmdir <path>` | Remove a remote directory. |

Example:

```text
sftp:/var/www> ls
sftp:/var/www> put ./dist/app.tar.gz /tmp/app.tar.gz
sftp:/var/www> get release.tar.gz ~/Downloads/
```

## Batch SFTP Commands

These commands do not enter the interactive shell and are suitable for scripts:

| Command | Usage | Description |
| --- | --- | --- |
| `knot sftp ls` | `knot sftp ls alias:/path` | List a remote directory. |
| `knot sftp stat` | `knot sftp stat alias:/path` | Show remote file or directory metadata. |
| `knot sftp rm` | `knot sftp rm alias:/path` | Remove a remote file. |
| `knot sftp mkdir` | `knot sftp mkdir alias:/path` | Create a remote directory, including missing parents. |
| `knot sftp rmdir` | `knot sftp rmdir alias:/path` | Remove a remote directory. |
| `knot sftp mv` | `knot sftp mv alias:/old alias:/new` | Rename a remote file or directory. Source and destination must use the same alias. |

Examples:

```sh
knot sftp ls web-prod:/var/www
knot sftp stat web-prod:/var/www/index.html
knot sftp mv web-prod:/tmp/a web-prod:/tmp/b
```

## `knot cp`

```sh
knot cp [SRC] [DEST] [flags]
```

| Argument | Required | Description |
| --- | --- | --- |
| `[SRC]` | Yes | Local path or remote path in `alias:/path` form. |
| `[DEST]` | Yes | Local path or remote path in `alias:/path` form. Remote-to-remote copy is not supported. |

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `-r, --recursive` | bool | `true` | Copy directories recursively. |
| `-f, --force` | bool | `false` | Overwrite existing files. |

Examples:

```sh
knot cp ./dist/. web-prod:/var/www/html/
knot cp web-prod:/var/log/nginx/access.log ./
```

When the source directory ends with `/.`, Knot copies the directory contents instead of the directory itself.
