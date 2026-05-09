# Config Sync

`knot sync` synchronizes server profiles, proxies, and managed keys through an encrypted remote archive. It is separate from `knot export` and `knot import`: sync is for day-to-day multi-device sharing, while import/export remains a full local backup and migration workflow.

WebDAV and S3-compatible providers are supported.

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
knot sync provider add s3
knot sync provider add s3 home
```

All of these forms can run interactively. If you only run `provider add`, Knot asks for the provider type first. Supported provider types are `webdav` and `s3`. If you pass the provider type, Knot starts from the alias prompt. If you pass the alias too, Knot starts from the provider fields. The first provider you add is set as the default automatically, so `knot sync push` and `knot sync pull` can be used without a provider alias.

For WebDAV scripts, pass the WebDAV fields as flags:

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

When the URL is treated as a directory, Knot uses `config.toml.enc` as the remote file name. Missing WebDAV directories are created before upload when the server supports `MKCOL`.

Examples:

| Input URL | Remote object |
| --- | --- |
| `https://dav.example.com/knot/config.toml.enc` | that exact file |
| `https://dav.example.com/knot/` | `https://dav.example.com/knot/config.toml.enc` |
| `https://dav.example.com/knot` | `https://dav.example.com/knot/config.toml.enc` |

For AWS S3:

```sh
knot sync provider add s3 home \
  --bucket my-bucket \
  --key knot/config.toml.enc \
  --region us-east-1 \
  --access-key-id "$S3_ACCESS_KEY_ID" \
  --secret-access-key "$S3_SECRET_ACCESS_KEY"
```

For S3-compatible services such as MinIO:

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

For services such as Cloudflare R2, pass the service endpoint and its expected signing region:

```sh
knot sync provider add s3 r2 \
  --endpoint https://<account-id>.r2.cloudflarestorage.com \
  --bucket knot \
  --region auto \
  --access-key-id "$R2_ACCESS_KEY_ID" \
  --secret-access-key "$R2_SECRET_ACCESS_KEY"
```

| Flag | Description |
| --- | --- |
| `--bucket` | S3 bucket. Required. |
| `--key` | S3 object key. Defaults to `config.toml.enc`. |
| `--region` | S3 signing region. Required. `auto` is only accepted with an explicit endpoint. |
| `--endpoint` | Optional S3-compatible endpoint URL. Leave empty for AWS S3. |
| `--access-key-id` | S3 access key ID. Stored encrypted in the local config. |
| `--secret-access-key` | S3 secret access key. Stored encrypted in the local config. |
| `--session-token` | Optional S3 session token. Stored encrypted in the local config. Use `-` with `provider edit` to clear it. |
| `--path-style` | Use path-style URLs. Leave it off for AWS S3 unless your endpoint requires it. |

By default, Knot uses virtual-hosted-style S3 URLs, where the bucket is part of the host name: `https://bucket.s3.region.amazonaws.com/key`. This is the recommended mode for AWS S3 and for compatible services that support bucket host names.

Enable `--path-style` only when your S3-compatible server expects the bucket in the path, for example `https://minio.example.com/bucket/key`. This is common for MinIO, local test servers, and deployments where wildcard DNS or bucket-specific TLS host names are not available. The setting only changes URL construction; signing still uses the configured `--region`, credentials, and endpoint.

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
| `provider list` | List providers in a table. WebDAV targets show the URL; S3 targets show `s3://bucket/key`. Alias: `provider ls`. |
| `provider show <alias>` | Show one provider without printing secrets. S3 credentials are shown only as `has_*` booleans. |
| `provider edit <alias>` | Edit a provider. With only an alias, it enters interactive mode. WebDAV and S3 edit flags match their add flags. |
| `provider remove <alias>` | Remove a provider. Aliases: `rm`, `delete`. |
| `provider set-default <alias>` | Store the default sync provider in `settings.default_sync_provider`. |
| `provider clear-default` | Clear the default sync provider. |

You can also set the default provider with:

```sh
knot config set default_sync_provider home
```

## Sync Password

The sync archive is encrypted with a sync password before it is uploaded. This password is independent from WebDAV passwords and S3 credentials. Provider credentials stay local and are not included in the sync archive.

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
