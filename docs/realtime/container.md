# Container

`mino serve` plus the HTTP API runs in a container. The repo ships a `Dockerfile` and a
`docker-compose.yaml`:

```sh
export MINO_HTTP_TOKEN=$(openssl rand -base64 32)
export MINO_GITHUB_SERVICE_TOKEN=ghp_…   # a machine-user PAT; needs repo read:org
docker compose up -d
curl -s localhost:7717/healthz
curl -s -H "Authorization: Bearer $MINO_HTTP_TOKEN" localhost:7717/api/v1/status | jq
```

The image is built with `SERVICE_AUTH=1`, so a configured service identity satisfies the
onboarding gate — a container has no human signing key. A stock `mino` binary is **not**
built that way, and will keep asking for onboarding even with a valid service credential.

Every config leaf takes a `MINO_*` override, so the image is configured entirely by
environment:

| Variable | Why it matters |
|---|---|
| `MINO_HOME` | Set to `/var/lib/mino` in the image. Must be set: with neither it nor `$HOME` resolvable, mino cannot pick a home. All five DuckDB files live in `$MINO_HOME/.data/` |
| `MINO_DAEMON_HTTP_HOST` | `0.0.0.0` in the image. Without this the listener is on the container's loopback and `-p 7717:7717` cannot reach it |
| `MINO_DAEMON_HTTP_TOKEN` | The bearer token, minimum 16 characters. Left unset, one is generated inside the volume where only `docker compose exec` can read it |
| `MINO_DAEMON_HTTP_ENABLED` / `_PORT` | On, `7717` |
| `MINO_GITHUB_SERVICE_TOKEN` | A machine-user PAT. Outranks the `gh` CLI and `$GITHUB_TOKEN`, and is what gives the container a realtime GitHub source |
| `MINO_GITHUB_APP_ID` / `_INSTALLATION_ID` / `_PRIVATE_KEY_PATH` | GitHub App installation auth. Mount the `.pem` as a file; see [Service authentication](../configuration/authentication.md#service-authentication) |
| `MINO_GITHUB_VIEWER` | The login that replaces `@me` in queries. A service identity is not you, so `author:@me` otherwise matches nothing |
| `GITHUB_TOKEN` / `GH_TOKEN` | Still honoured, below service auth. A human's token, with a human's rate limit and name on every action |
| `MINO_LOG_DIR` | Optional; otherwise logs go to `$MINO_HOME/logs` |

Env var names are matched exactly. `MINO_GITHUB_APPID` and `MINO_GITHUB_PRIVATE_KEY` resolve
to nothing and are silently ignored.

**Binding off-loopback removes a defence, it does not add one.** On a desktop the bind
address is loopback and a non-loopback `Host` header is rejected outright, so the bearer
token is the *second* of two controls. `MINO_DAEMON_HTTP_HOST=0.0.0.0` removes the first:
the token is then the **only** thing between the network and endpoints that run flights, run
saved queries, and execute plugin actions against your GitHub credentials. Generate at least
32 random bytes, keep it out of shell history and out of `docker inspect`-visible build args,
publish the port to `127.0.0.1` on the host unless you have a reason not to, and put a
TLS-terminating proxy in front of it if it must cross a network — the API speaks plaintext
HTTP, so the token travels in the clear. serve prints a warning on startup whenever the bind
is not loopback.

Three constraints are worth knowing before you deploy it:

- **One container per volume.** DuckDB allows a single read-write process per file, so a
  second mino against the same `.data/` fails with a lock error. There is no leader election
  and no shared-state story; `deploy.replicas: 2` is a footgun.
- **No OS keyring, and service auth does not need one.** The App private key is a mounted
  file and the service PAT is an env var, so neither touches the sealed store — that is the
  point. What still does not work: `mino login` cannot persist a credential, cached OAuth
  tokens are unreadable, and encrypted backups cannot resolve a key, all because
  `tokens.duckdb` is sealed with a key escrowed in the D-Bus Secret Service. The Google
  signals in the overlay need `gcloud` ADC or a browser OAuth flow and will not authenticate.
- **A GitHub App does not give you realtime GitHub.** `GET /notifications` lists notifications
  *for the authenticated user*, and an App installation token has no user — GitHub answers
  `403 Resource not accessible by integration`. The notification poller is mino's only
  realtime GitHub source, so realtime needs a **machine-user PAT**
  (`MINO_GITHUB_SERVICE_TOKEN`), not an App. serve refuses the combination at startup with a
  message naming the fix rather than failing every poll. An App is still the better identity
  for the fetch paths: flights, saved queries, projects, Actions and detail views.
- **A realtime source must open, or serve exits.** With no GitHub credential the container
  starts, provisions, and then exits. As of this change the error lists *which* query was
  skipped and why, instead of only saying the flight has no realtime signals.

On first run the entrypoint runs `mino install` when `$MINO_HOME` has no `config.yaml`,
because `mino serve` errors against an unprovisioned home and nothing auto-provisions. It
never passes `--force`, so your edited config survives restarts. The image runs as uid
`10001`; an **empty named volume** inherits that ownership automatically, but a **bind
mount** does not — `chown -R 10001:10001` it on the host first, or you will see a confusing
DuckDB open failure rather than a permission error.

The image is Debian-based, not distroless: mino links DuckDB through cgo and needs
`libstdc++6` and glibc at runtime, so `scratch` and `distroless/static` cannot run it. The
builder and runtime images must stay on the same Debian release, since the builder's glibc
sets the binary's symbol floor.
