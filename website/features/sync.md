# Config Sync

`knot sync` synchronizes server profiles, proxies, and managed keys through an encrypted remote archive. It is separate from `knot export` and `knot import`: sync is for day-to-day multi-device sharing, while import/export remains a full local backup and migration workflow.

Only WebDAV providers are supported currently.

## What Is Synced

Sync archives include:

| Section | Synced |
| --- | --- |
| `servers` | yes |
| `proxies` | yes |
| `keys` | yes |
| `settings` | no |
| `sync_providers` | no |
| daemon state, logs, known hosts | no |

This keeps machine-local preferences and provider credentials on each device.

## Provider Setup

```sh
knot sync provider add
knot sync provider add webdav
knot sync provider add webdav home
```

All of these forms can run interactively. If you only run `provider add`, Knot asks for the provider type first. The current provider type is `webdav`. If you pass `webdav`, Knot starts from the alias prompt. If you pass the alias too, Knot starts from the WebDAV fields.

For scripts, pass the WebDAV fields as flags:

```sh
knot sync provider add webdav home \
  --url https://dav.example.com/knot/ \
  --user alice \
  --password "$WEBDAV_PASSWORD"
```

| Flag | Description |
| --- | --- |
| `--url` | WebDAV URL. A URL ending in a file-like path is used as-is. Other URLs are treated as directories. |
| `--user` | WebDAV username. |
| `--password` | WebDAV password. It is stored encrypted in the local config. |

When the URL is treated as a directory, Knot uses `config-sync.toml.enc` as the remote file name. Missing WebDAV directories are created before upload when the server supports `MKCOL`.

Examples:

| Input URL | Remote object |
| --- | --- |
| `https://dav.example.com/knot/config-sync.toml.enc` | that exact file |
| `https://dav.example.com/knot/` | `https://dav.example.com/knot/config-sync.toml.enc` |
| `https://dav.example.com/knot` | `https://dav.example.com/knot/config-sync.toml.enc` |

## Provider Commands

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

| Command | Description |
| --- | --- |
| `provider list` | List providers in a table. Alias: `provider ls`. |
| `provider show <alias>` | Show one provider without printing secrets. |
| `provider edit <alias>` | Edit a provider. With only an alias, it enters interactive mode. |
| `provider remove <alias>` | Remove a provider. Aliases: `rm`, `delete`. |
| `provider set-default <alias>` | Store the default sync provider in `settings.default_sync_provider`. |
| `provider clear-default` | Clear the default sync provider. |

You can also set the default provider with:

```sh
knot config set default_sync_provider home
```

## Sync Password

The sync archive is encrypted with a sync password before it is uploaded. This password is independent from the WebDAV password.

```sh
knot sync password set
knot sync password set --password-stdin
knot sync password status
knot sync password clear
```

If no sync password is saved, `push` and `pull` ask for it interactively. In scripts, use `--password-stdin`.

## Push

```sh
knot sync push
knot sync push home
knot sync push --provider home
```

`push` exports the local `servers`, `proxies`, and `keys`, encrypts them with the sync password, then uploads the archive to the selected provider. In an interactive terminal it asks before overwriting the remote archive unless `--force` is used.

| Flag | Description |
| --- | --- |
| `--provider <alias>` | Select a provider. This overrides the positional provider and the default provider. |
| `--password-stdin` | Read the sync password from stdin. |
| `--no-save-password` | Do not save the sync password from this run. |
| `--force` | Skip the overwrite confirmation. |

## Pull

```sh
knot sync pull home --strategy local-first
knot sync pull --provider home --strategy remote-first
knot sync pull home --strategy overwrite --dry-run
```

`pull` downloads the encrypted archive, decrypts it, then merges the remote `servers`, `proxies`, and `keys` into the local config. Local `settings` and `sync_providers` are always preserved.

| Flag | Description |
| --- | --- |
| `--provider <alias>` | Select a provider. |
| `--strategy <name>` | Merge strategy: `local-first`, `remote-first`, or `overwrite`. |
| `--password-stdin` | Read the sync password from stdin. |
| `--dry-run` | Show the merge summary without writing the local config. |
| `--force` | Skip confirmation prompts where applicable. |

In non-interactive mode, pass `--strategy` explicitly.

## Merge Strategies

| Strategy | Behavior |
| --- | --- |
| `local-first` | Match by alias. Local items win conflicts; remote-only items are added. |
| `remote-first` | Match by alias. Remote items win conflicts; local-only items are kept. |
| `overwrite` | Replace local `servers`, `proxies`, and `keys` with the remote archive. Local `settings` and `sync_providers` stay local. |

Knot remaps internal IDs during merge so server references to keys, proxies, and jump hosts continue to point at the final kept objects.

