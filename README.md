# Mino shift assistant

[![CI](https://github.com/codyconfer/mino/actions/workflows/ci.yml/badge.svg)](https://github.com/codyconfer/mino/actions/workflows/ci.yml)

Mino is a command-line assistant for the signals you check at the start of — and
throughout — an SRE shift. Stock Mino ships **GitHub**, **GitLab**, **Gitea/Forgejo**, and
**notes / tasks / reminders**; **Google** (Calendar / Gmail / Docs / Drive / Tasks), **Slack**, and the
demo signal live in [`external/plugins`](external/plugins/) and are linked by an
overlay binary. Query a single signal ad-hoc, save reusable **queries** and
**filters** and recall them by name, or send Mino on a named **flight** that fetches
a whole set concurrently. Results print as terminal panels or JSON — or through a
**formatter**, a template that turns a run into a report you can paste.

The name is for the bird — *Mino*, the genus of golden mynas — and is pronounced
like the "mino bird": **MY-noh**, rhyming with "rhino".

## Getting started

Build (or install) the binary:

```sh
go build -o mino .
# or: go install github.com/codyconfer/mino@latest
```

Bootstrap a config directory (with a sample query, filter, and flight), then run:

```sh
mino install                 # create ~/.mino with defaults
mino onboard                 # one-time: git provider auth + a provider-verified GPG key
mino fly                     # run the default flight
mino github query            # ad-hoc: your open PRs + review requests
mino gitlab query            # the same for GitLab merge requests
mino fly default -o json | jq .
```

Or run `serve` plus its HTTP trigger API as a container service:

```sh
export MINO_HTTP_TOKEN=$(openssl rand -base64 32)
export MINO_GITHUB_SERVICE_TOKEN=ghp_…   # a machine-user PAT, not your own token
export MINO_GITHUB_VIEWER=acme-bot       # what @me should mean for a service identity
docker compose up -d                     # API on 127.0.0.1:7717/api/v1
```

See [Container](docs/realtime/container.md) — note that binding off-loopback leaves the
bearer token (or an allow-listed GitHub sign-in) as the only control on endpoints that run
flights and plugin actions.
mino can also authenticate to GitHub as a service rather than as you: see
[Service authentication](docs/configuration/authentication.md#service-authentication).

On first use Mino guides you through onboarding — git provider auth plus a provider-verified
signing key — and how strictly it gates depends on the mode; see
[Onboarding](docs/configuration/onboarding.md). Authentication reuses tools you already have
(the `gh` CLI, `gcloud` ADC, `$SLACK_TOKEN`) and falls back to `mino login <service>`
when they are absent; see [Authentication](docs/configuration/authentication.md) for the
resolution order and required scopes. For unattended deployments a GitHub App or a
machine-user PAT can take over instead — see
[Service authentication](docs/configuration/authentication.md#service-authentication).

## Modes

Four modes over the same engine:

| Mode | Command | What it does |
|---|---|---|
| **cli** | `mino <directive>` | Run a directive and print the result |
| **serve** | `mino serve [flight]` | Foreground realtime watcher in the current shell; `--http` adds a loopback trigger API |
| **daemon** *(experimental, `-tags daemon`)* | `mino daemon [flight]` | Install (if needed) then start the OS service |
| **deck** | `mino deck [flight]` | Full-screen interactive TUI |

→ [Operating modes](docs/operating-modes.md) for the stdio and logging contracts,
`make` targets, and the tmux workspace.

## Quick reference

| Command | Does |
|---|---|
| `mino fly [flight]` | Run a named flight |
| `mino query [name]` | Run a saved query, or list them |
| `mino <signal> query` | Ad-hoc single-signal query |
| `mino deck [flight]` | Open the interactive TUI |
| `mino serve [flight]` | Foreground realtime watcher |
| `mino list [kind]` | List what the active role can see |
| `mino formatter [name]` | List formatters, or show one |
| `mino verify [target]` | Validate config and directives |
| `mino history` | List past runs |
| `mino config` | Show the active config |
| `mino role` | Show the active and defined roles |
| `mino login <service>` | OAuth login for github, plus any provider a plugin contributes |
| `mino install` | Create the config directory with defaults |
| `mino onboard` | One-time setup gate |

Common flags: `-o json` (pipeable output), `-F <name>` (render through a formatter),
`--role <name>`, `--home <dir>`, `-v` (debug logging).

`mino serve --http` also exposes a token-guarded HTTP API under `/api/v1` — loopback by
default — that triggers flights, queries and actions and streams events over SSE, as an
alternative to typing the command. Callers can optionally sign in with a GitHub identity
instead of sharing the token. See [HTTP trigger API](docs/realtime/http-api.md).

→ [Command reference](docs/reference/commands.md) ·
[Common flags](docs/reference/commands.md#common-flags)

## Documentation

The full manual lives under **[docs/](docs/readme.md)**, which indexes every page:

- [Getting started](docs/getting-started.md) — install, bootstrap, and the pipeline
- [Operating modes](docs/operating-modes.md) — cli, serve, daemon, deck; the tmux workspace
- [Queries and filters](docs/directives/queries-and-filters.md) — signals, params, regex rules
- [Flights](docs/directives/flights.md) — named concurrent sweeps
- [Roles](docs/directives/roles.md) — scope what Mino shows
- [Formatters](docs/directives/formatters.md) — templated reports over a run
- [Realtime: serve & daemon](docs/realtime/serve-and-daemon.md) — active signals, notifications
- [HTTP trigger API](docs/realtime/http-api.md) — `/api/v1` triggers and the SSE stream
- [Container](docs/realtime/container.md) — image, compose, and the `0.0.0.0` caveat
- [Service authentication](docs/configuration/authentication.md#service-authentication) — GitHub App / machine-user auth
- [Configuration](docs/configuration/readme.md) — config dir layout, DuckDB store, logs
- [Data signals](docs/reference/signals.md) — what each signal reads and writes
- [Command reference](docs/reference/commands.md) — every command and common flag
- [Plugins & notes](docs/plugins.md) — the plugin model and the overlay binary
- [Development & internals](docs/development.md) — build, test, build tags

Copy-paste starters live in [`examples/`](examples/).

## License

Copyright (c) 2026 Cody Confer. Licensed under the GNU Affero General Public License
v3.0 — see [LICENSE](LICENSE).

Mino depends on [sisyphus](https://github.com/codyconfer/sisyphus) and
[viewkit](https://github.com/codyconfer/viewkit), both MIT.
