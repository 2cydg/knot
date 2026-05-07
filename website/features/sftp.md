# SFTP and File Copy

Knot provides two styles of file operation: the interactive `knot sftp` shell and script-friendly batch commands. `knot cp` uses Docker-style `alias:/path` remote paths for copying between the local machine and one remote server.

## Interactive SFTP

```sh
knot sftp [alias] [remote_path]
knot sftp [alias] --follow
```

| Argument | Required | Description |
| --- | --- | --- |
| `[alias]` | Yes | Server alias. |
| `[remote_path]` | No | Initial remote directory. |

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--follow` | bool | `false` | Follow the current directory of an active `knot ssh` session for the same alias. Cannot be used with `[remote_path]`. |

Examples:

```sh
knot sftp web-prod
knot sftp web-prod /var/www
knot sftp web-prod --follow
```

The interactive shell supports command and path completion. Remote `~` resolves to the connected user's home directory. If `settings.default_sftp_local_path` is configured, relative local paths in the REPL are resolved from that directory, and local Tab completion starts there.

## Follow SSH Directory

`knot sftp web-prod --follow` opens an interactive SFTP shell that follows an existing `knot ssh web-prod` session. If one SSH session is active, Knot follows it automatically. If several sessions are active, Knot lists numbered sessions with start times and known directories, then prompts you to enter the `No.` to follow.

When the followed SSH session reports a new directory, the SFTP shell tries to switch to the same path and prints a short `[follow]` message. If the directory cannot be opened through SFTP, the shell keeps its current directory and waits for the next update. If the followed SSH session closes, SFTP stays open in the last directory and stops following.

Directory follow relies on OSC 7 current-directory escape sequences. When `knot sftp --follow` attaches to an active SSH session, Knot injects a temporary OSC 7 hook for Bash and Zsh into that followed session. Other shells work only if they already emit OSC 7 themselves. If no OSC 7 sequence is emitted, Knot cannot track directory changes.

The injected hook is sent as keyboard input to the followed SSH session and ends with Enter. Use `--follow` when the followed session is sitting at a normal shell prompt. Do not start follow while the session is focused inside editors or full-screen programs such as Vim, less, top, database shells, or language REPLs, because the hook text may be handled by that foreground program. The hook is temporary and does not modify `.bashrc`, `.zshrc`, or other remote shell configuration files.

## SFTP Shell Commands

| Command | Usage | Description |
| --- | --- | --- |
| `help` | `help` or `?` | Show help. |
| `clear` | `clear` | Clear the current terminal output. |
| `exit` | `exit`, `quit`, or `bye` | Exit the SFTP shell. |
| `ls` | `ls [path]` | List remote directory contents. |
| `pwd` | `pwd` | Print the current remote directory. |
| `cd` | `cd <path>` | Change remote directory. `cd ~` jumps to the remote home directory. |
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
sftp:/var/www> clear
sftp:/var/www> cd ~
sftp:/var/www> put ./dist/app.tar.gz /tmp/app.tar.gz
sftp:/var/www> get release.tar.gz ~/Downloads/
```

## Local Base Directory

Set a default local directory for the interactive SFTP shell:

```sh
knot config set default_sftp_local_path ~/Downloads
```

With `default_sftp_local_path` configured:

- `get release.tar.gz` downloads to `<default_sftp_local_path>/release.tar.gz`.
- `get -r logs` downloads to `<default_sftp_local_path>/logs`.
- `put app.tar.gz /tmp/` reads `<default_sftp_local_path>/app.tar.gz`.
- `mget` and `mput` use the same local base directory for omitted relative paths.
- Absolute local paths, `~/...`, and Windows drive paths still behave as explicit paths and are not rebased.

## Remote Path Rules

- Remote absolute paths such as `/var/www` are used as-is.
- Remote relative paths are resolved from the current SFTP directory.
- `~` means the remote user's home directory.
- `~/logs` means `logs` under the remote home directory.

## Upload Rules

These rules apply to both interactive `put` and batch `knot cp` uploads.

### Uploading Files

Assume the current remote directory is `/cwd` and the local source is `file.txt`.

| Command | Result |
| --- | --- |
| `put file.txt` | Uploads to `/cwd/file.txt`. |
| `put file.txt remote.txt` | Uploads as `/cwd/remote.txt` if it does not exist as a directory. |
| `put file.txt remote/` | Treats `remote/` as a directory and uploads to `/cwd/remote/file.txt`. Missing directories are created automatically. |
| `put file.txt existing_dir` | If `existing_dir` is a remote directory, uploads to `/cwd/existing_dir/file.txt`. |
| `put file.txt/` | Returns a clear error because the local source is a file, not a directory. |

### Uploading Directories

Assume the current remote directory is `/cwd` and the local source is `dist`.

| Command | Result |
| --- | --- |
| `put -r dist` | Uploads the directory itself to `/cwd/dist`. |
| `put -r dist/` | Same as `put -r dist`. A trailing slash does not change the meaning. |
| `put -r dist/.` | Uploads the contents of `dist` into `/cwd` without creating an extra `/cwd/dist`. |
| `put -r dist release` | If `release` does not exist, uploads the directory as `/cwd/release`. If `release` is a directory, uploads to `/cwd/release/dist`. |
| `put -r dist release/` | Treats `release/` as a container directory and uploads to `/cwd/release/dist`. Missing directories are created automatically. |
| `put -r dist/. release` | Uploads the contents of `dist` into `/cwd/release`. Missing directories are created automatically. |
| `put -r dist existing_file` | Returns a clear error because the target already exists as a file. |

## Download Rules

Assume the current remote directory is `/cwd` and the remote source is `release.tar.gz` or `logs`.

| Command | Result |
| --- | --- |
| `get release.tar.gz` | Downloads to `./release.tar.gz`, or to `<default_sftp_local_path>/release.tar.gz` if configured. |
| `get release.tar.gz out.tar.gz` | Downloads as `out.tar.gz`. |
| `get release.tar.gz target/` | Treats `target/` as a directory and downloads to `target/release.tar.gz`. Missing directories are created automatically. |
| `get -r logs` | Downloads the directory itself to `./logs`, or to `<default_sftp_local_path>/logs` if configured. |
| `get -r logs/` | Same as `get -r logs`. A trailing slash does not change the meaning. |
| `get -r logs/.` | Downloads the contents of `logs` into the local target directory without creating an extra `logs` directory. |
| `get -r logs archive` | If `archive` does not exist, downloads the directory as `archive`. If it exists and is a directory, downloads to `archive/logs`. |

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

For `knot cp`, `dist` and `dist/` are equivalent and copy the directory itself. Only `dist/.` copies the directory contents without the extra top-level directory.
