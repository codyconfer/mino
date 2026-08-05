# Encrypted backups

`mino backup` bundles the DuckDB databases (`config`, `audit`, `tokens`) into a
tar and encrypts it with AES-256-GCM. The key is escrowed in a **secret
manager** — `secret_backend: auto` uses the **Bitwarden** (`bw`) or **1Password**
(`op`) CLI when configured, otherwise the **OS keyring**; if none is available it
errors rather than writing an unrecoverable backup.

```sh
mino backup                 # → ./mino-backup-<ts>.tar.enc  (key escrowed)
mino backup --out /secure   # write elsewhere
mino restore <file>         # decrypt + write the databases back into <home>/.data
```

`backup.keep: N` retains only the newest N backups (`0` = keep all).
`backup.destination` accepts `local` (the current directory) or the name of a
plugin-contributed destination: with the overlay's Drive plugin registered,
`gdrive` uploads the encrypted file to the app's private Google Drive
`appDataFolder`. An unknown destination names the ones actually registered.
`mino restore`
doesn't depend on opening `.data/config.duckdb`, so it recovers even a corrupted config
DB.
