# Authentication

Every signal resolves auth as **CLI/ADC → token → OAuth**, so it works whether or
not the usual CLI is installed; when nothing is configured Mino explains the
options instead of failing opaquely.

| Signal | Primary | Fallbacks |
|---|---|---|
| **GitHub** (stock) | `github.app` / `github.service_token` (service identity) | `gh` CLI (`gh auth login`) → `$GITHUB_TOKEN` / `$GH_TOKEN` → `mino login github` (device flow) |
| **Gitea / Forgejo** (stock) | `gitea.service_token` (service identity) | `$GITEA_TOKEN` / `$FORGEJO_TOKEN` → `mino login gitea` (paste a token) |
| **Calendar / Gmail / Docs / Drive / Tasks** (overlay) | `gcloud` ADC | `mino login google` (browser OAuth) |
| **Slack** (overlay) | `$SLACK_TOKEN` (xoxp-…) | `mino login slack` (browser OAuth) |

`mino login <provider>` runs that provider's login flow and caches a token in the
DuckDB credential store (`.data/tokens.duckdb`, one row per service); later runs
use the signal's direct API client. Stock mino ships `github` and `gitea` (with
`forgejo` as a second name for the same implementation);
`google` and `slack` are contributed by the overlay plugins through
`plugin.RegisterLoginProvider`, along with the signal aliases (`mino login
calendar` → Google). Each needs its OAuth app credentials in config — GitHub under
`github.oauth_client_id`, contributed providers under
`plugins.<namespace>.oauth_client_id` / `_secret`. GitHub uses the device flow (no
secret), Google and Slack use a localhost browser-redirect flow, and Google tokens
auto-refresh.

- **GitHub Enterprise** — set `github.api_url` (e.g.
  `https://ghe.example.com/api/v3`) so the REST fallback targets your instance.
  Device-flow scopes are `github.oauth_scopes` (default `repo read:org`); they apply to the
  device flow only, and a GitHub App uses installation permissions instead.
- **Google scopes** — a plain `gcloud auth application-default login` does *not*
  grant the read scopes. Mino preflight-checks them and reprints the exact
  `gcloud … --scopes=…` command to run if any are missing.
- **Slack scopes** — `mino login slack` now requests `im:history`, `im:read`,
  `mpim:history`, `mpim:read`, `search:read` and `users:read` on top of the original
  channel scopes. Only the new read surfaces need them: `slack query --channel`
  keeps working on a token minted before they existed. `--mentions`, `--search` and
  `--dms` need `search:read` / the `im`+`mpim` scopes, and display-name resolution
  needs `users:read` — without it items fall back to raw `U…` ids rather than
  failing. Re-run `mino login slack` to pick the new scopes up, or pin your own set
  with `plugins.slack.user_scopes`.
- **Slack streaming** — `serve` needs an app-level token (`$SLACK_APP_TOKEN`,
  `xapp-…`, with `connections:write`) *and* a bot token (`$SLACK_BOT_TOKEN`,
  `xoxb-…`) for Socket Mode; the user token alone only covers queries.

## Git providers

The auth checks mino performs — is the credential live, is your signing key registered on
the forge, is your commit email verified, what is the account login and rate limit — go
through a **provider** interface (`internal/gitauth`), not through GitHub directly. The
stock binary ships two, `github` and `gitea` (plus `forgejo`, the same implementation
under a second name), and `git.provider` selects one:

```yaml
git:
  provider: github     # the default; MINO_GIT_PROVIDER also works
```

A provider registers itself with `gitauth.Register(name, factory)` and reads its own
settings through `Env.Get(key)`, so adding a forge needs no change in `internal/app` or
`internal/config`. GitHub reads `api_url`, `service_token`, `app.id` and friends from the
typed `github:` section; a provider contributed by a plugin reads the same-shaped keys from
its own `plugins.<provider>:` namespace. `mino verify auth` names the active provider, and
an unknown `git.provider` fails with the list of registered names.

Each provider brings its own signal: `github query` and `gitea query` are separate
signals with separate query languages, because the two forges do not search alike.
GitHub-specific features stay GitHub-specific — project boards and Actions have no Gitea
equivalent in mino.

### Gitea and Forgejo

```yaml
git:
  provider: gitea            # or forgejo; both read the gitea: section below
gitea:
  url: https://git.example.com   # required — mino appends /api/v1
  # api_url: https://git.example.com/gitea/api/v1   # only for an unusual path prefix
  max: 30
  queries:
    - "type:pulls state:open created:@me"
```

- **`gitea.url` is required.** Gitea is always self-hosted, so there is no endpoint to
  default to; leaving it unset is a startup error naming the field rather than a quiet
  failure. Plain `http` is accepted for `localhost` and other loopback addresses, so a
  throwaway instance works, and refused everywhere else so a token never crosses a network
  in the clear.
- **`gitea` and `forgejo` share everything but the label** — one config section, one pair of
  credential-store keys (`gitea`, `gitea-service`), one set of `MINO_GITEA_*` variables.
  There are no `MINO_FORGEJO_*` variables.
- **The auth scheme is `token`, not `Bearer`.** Gitea has accepted `Authorization: token
  <pat>` on every version and `Bearer` only on recent ones, so mino sends the portable one.
- **Token scopes** are coarse: `read:user` covers the account, e-mail and key checks,
  `read:repository` and `read:issue` cover queries and item detail, and `read:notification`
  covers the realtime stream. Create tokens at `<instance>/user/settings/applications`.
- **No rate limit is reported.** Gitea exposes no `/rate_limit` endpoint, so the status strip
  shows the account with no `n/m` counter rather than inventing `0/0`, which would read as
  throttled.
- **One key for access and signing.** Gitea verifies commit signatures against the keys at
  `<instance>/user/settings/keys`; the split GitHub makes between `user/keys` and
  `user/ssh_signing_keys` does not exist there, so any registered key counts.
- **Version drift shows up as a 404.** Gitea answers 404 both for an endpoint a version does
  not have and for a scope a token was not granted, so mino's hint names both and points at
  `<instance>/api/swagger`.
- **The `tea` CLI is deliberately not a tier**, unlike `gh` for GitHub: it would make a
  machine's identity depend on what happens to be on `PATH`, and it holds no notion of the
  instance mino is configured against.

## Service authentication

For unattended deployments mino can authenticate to GitHub as a **service** rather than as
you — either a GitHub App installation or a machine-user PAT. Both outrank everything ambient:

```
github.app  →  github.service_token  →  gh CLI  →  $GITHUB_TOKEN  →  $GH_TOKEN  →  mino login github
gitea.service_token  →  sealed store "gitea-service"  →  $GITEA_TOKEN  →  $FORGEJO_TOKEN  →  mino login gitea
```

```yaml
github:
  viewer: acme-bot           # replaces @me in queries; a service identity is not you
  # service_token: ghp_…     # or MINO_GITHUB_SERVICE_TOKEN, or the sealed store key
                             # "github-service"
  # app:
  #   id: "123456"
  #   installation_id: "78901234"   # omit and mino discovers it if there is exactly one
  #   private_key_path: /run/secrets/mino-github-app.pem
```

Exact env var names: `MINO_GITHUB_SERVICE_TOKEN`, `MINO_GITHUB_VIEWER`,
`MINO_GITHUB_APP_ID`, `MINO_GITHUB_APP_INSTALLATION_ID`, `MINO_GITHUB_APP_PRIVATE_KEY_PATH`,
and `MINO_GITHUB_APP_PRIVATE_KEY` for an inline PEM (raw or base64). For Gitea:
`MINO_GITEA_URL`, `MINO_GITEA_API_URL`, `MINO_GITEA_SERVICE_TOKEN`, `MINO_GITEA_VIEWER`.
Anything else — say `MINO_GITHUB_APPID`, or `MINO_GITEA_TOKEN` — resolves to nothing and is
silently ignored.

**There is no `gitea.token` config key either**, for the same reason as
`github.app.private_key` below: `mino login gitea` prompts for a personal access token and
seals it in the credential store, and with no config field to bind to, the token cannot be
reflected back out through `/api/v1/config` or `mino config export`. mino proves a pasted
token with one `GET /user` before sealing it, so a typo fails immediately instead of
becoming the top-ranked credential.

**Configuring a service identity is a commitment, not a preference.** Once `github.app` or
`github.service_token` is set, mino will not fall back to your personal credential: a
half-configured App is a startup error naming the missing field, not a quiet downgrade. mino
writes — actions, comments, workflow re-runs — and a silent fallback would land those under
your name and move rate-limit accounting to your account.

**The private key never belongs in config.** There is no `github.app.private_key` field, by
design: with nothing for the env overlay to bind to, key material cannot be reflected back out
of the loaded config, so `/api/v1/config`, `mino config export` and `mino verify` snippets are
safe by construction rather than by remembering to redact. Mount the `.pem` as a file and point
`private_key_path` at it. Prefer that over `MINO_GITHUB_APP_PRIVATE_KEY`, whose value is
visible to `docker inspect` and lands in shell history.

**Apps have permissions, not scopes.** `repo` maps to Contents / Pull requests / Issues read,
`read:org` to Members read, `read:project` to Projects read, and `mino github --actions` needs
Actions read. `gh auth refresh -s …` does not apply to an App at all.

Two limits worth knowing before you choose:

- **An App cannot read `/notifications`**, so it drives no realtime source. Use a machine-user
  PAT for the notification stream; serve refuses App-only realtime at startup and says so.
- **`@me` means nothing to a service identity.** Set `github.viewer` to the login mino should
  stand in for. Without it, `author:@me` matches nothing and queries return empty *with no
  error* — mino warns at build time and `mino verify auth` reports it.

Whether a service identity satisfies the onboarding gate is a **compile-time** decision:
`make build SERVICE_AUTH=1`. The default is off, so nothing reachable at runtime — no config
value, env var, or credential — can opt a machine out of the commit-signing requirement. The
container image sets it; the stock binary does not.

`mino verify auth` reports the selected mechanism, validates the App id, installation id and
key file, mints a token to prove the installation works, and never prints key material. Run
it with `-v` to see the full resolution trace, including which tiers lost and why.
