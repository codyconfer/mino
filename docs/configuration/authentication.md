# Authentication

Every signal resolves auth as **CLI/ADC → token → OAuth**, so it works whether or
not the usual CLI is installed; when nothing is configured Mino explains the
options instead of failing opaquely.

| Signal | Primary | Fallbacks |
|---|---|---|
| **GitHub** (stock) | `github.app` / `github.service_token` (service identity) | `gh` CLI (`gh auth login`) → `$GITHUB_TOKEN` / `$GH_TOKEN` → `mino login github` (device flow) |
| **Calendar / Gmail / Docs / Drive / Tasks** (overlay) | `gcloud` ADC | `mino login google` (browser OAuth) |
| **Slack** (overlay) | `$SLACK_TOKEN` (xoxp-…) | `mino login slack` (browser OAuth) |

`mino login <provider>` runs that provider's OAuth flow and caches a token in the
DuckDB credential store (`.data/tokens.duckdb`, one row per service); later runs
use the signal's direct API client. Stock mino ships the `github` provider only;
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

## Git providers

The auth checks mino performs — is the credential live, is your signing key registered on
the forge, is your commit email verified, what is the account login and rate limit — go
through a **provider** interface (`internal/gitauth`), not through GitHub directly. The
stock binary ships one provider, `github`, and `git.provider` selects it:

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

What is *not* abstracted: the GitHub signal itself. Flights, saved queries, projects,
Actions and the notification stream are still GitHub-specific, so a second forge needs its
own signal alongside its provider.

## Service authentication

For unattended deployments mino can authenticate to GitHub as a **service** rather than as
you — either a GitHub App installation or a machine-user PAT. Both outrank everything ambient:

```
github.app  →  github.service_token  →  gh CLI  →  $GITHUB_TOKEN  →  $GH_TOKEN  →  mino login github
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
and `MINO_GITHUB_APP_PRIVATE_KEY` for an inline PEM (raw or base64). Anything else — say
`MINO_GITHUB_APPID` — resolves to nothing and is silently ignored.

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
