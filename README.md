# Mino shift assistant

[![CI](https://github.com/codyconfer/mino/actions/workflows/ci.yml/badge.svg)](https://github.com/codyconfer/mino/actions/workflows/ci.yml)

Mino is a command-line assistant for the signals you check at the start of — and
throughout — an SRE shift. Stock Mino ships **GitHub** plus **notes / tasks /
reminders**; **Google** (Calendar / Gmail / Docs / Drive / Tasks), **Slack**, and the
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
mino onboard                 # one-time: GitHub auth + a GitHub-verified GPG key
mino fly                     # run the default flight
mino github query            # ad-hoc: your open PRs + review requests
mino fly default -o json | jq .
```

On first use Mino guides you through onboarding — GitHub auth plus a GitHub-verified
signing key — and how strictly it gates depends on the mode; see
[Onboarding](docs/readme.md#onboarding). Authentication reuses tools you already have
(the `gh` CLI, `gcloud` ADC, `$SLACK_TOKEN`) and falls back to `mino login <service>`
when they are absent; see [Authentication](docs/readme.md#authentication) for the
resolution order and required scopes.

## Modes

Four modes over the same engine:

| Mode | Command | What it does |
|---|---|---|
| **cli** | `mino <directive>` | Run a directive and print the result |
| **serve** | `mino serve [flight]` | Foreground realtime watcher in the current shell |
| **daemon** *(experimental, `-tags daemon`)* | `mino daemon [flight]` | Install (if needed) then start the OS service |
| **deck** | `mino deck [flight]` | Full-screen interactive TUI |

→ [Operating modes](docs/readme.md#operating-modes) for the stdio and logging contracts,
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

→ [Command reference](docs/readme.md#command-reference) ·
[Common flags](docs/readme.md#common-flags)

## Documentation

The full manual lives in **[docs/readme.md](docs/readme.md)**:

- [Queries and filters](docs/readme.md#queries-and-filters) — signals, params, regex rules
- [Flights](docs/readme.md#flights) — named concurrent sweeps
- [Roles](docs/readme.md#roles) — scope what Mino shows
- [Formatters](docs/readme.md#formatters) — templated reports over a run
- [Realtime: serve & daemon](docs/readme.md#realtime-serve--daemon) — active signals, notifications
- [Configuration](docs/readme.md#configuration) — config dir layout, DuckDB store, logs
- [Data signals](docs/readme.md#data-signals) — what each signal reads and writes
- [Development & internals](docs/readme.md#development--internals) — build, test, plugins

Copy-paste starters live in [`examples/`](examples/).

## License

Copyright (c) 2026 Cody Confer. Licensed under the GNU Affero General Public License
v3.0 — see [LICENSE](LICENSE).

Mino depends on [sisyphus](https://github.com/codyconfer/sisyphus) and
[viewkit](https://github.com/codyconfer/viewkit), both MIT.
