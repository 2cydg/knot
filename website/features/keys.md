# SSH Keys

`knot key` manages SSH private keys stored by Knot. Server profiles reference managed keys with `--key`.

## Commands

```sh
knot key [command]
```

| Command | Description |
| --- | --- |
| `knot key list` | List managed keys. Alias: `knot key ls`. |
| `knot key add [alias]` | Add or overwrite a managed key. |
| `knot key edit [alias]` | Replace the content of an existing key. |
| `knot key remove [alias]` | Remove a managed key. Alias: `knot key rm`. |

## Add a Key

```sh
knot key add [alias] [flags]
```

| Flag | Type | Default | Description |
| --- | --- | --- | --- |
| `--file` | string | empty | Private key file path. |
| `--content` | string | empty | Private key content. |
| `--passphrase` | string | empty | Passphrase for an encrypted private key. It may be visible in process lists. |

Examples:

```sh
knot key add deploy --file ~/.ssh/id_ed25519
knot key add deploy
```

Interactive mode is usually safer when passphrases are involved.

## Use a Key from a Server

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy
knot edit web-prod --key deploy
```

When a key alias is set, Knot switches the auth method to key if needed.

## Remove a Key

```sh
knot key remove deploy
knot key rm deploy
```

If servers reference the key, Knot asks before clearing those references.
