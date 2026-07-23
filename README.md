# Munin SRE shift assistant

> In Norse myth Odin's raven **Muninn** ("memory") flies out over the world each
> day and returns to whisper what it saw. This Munin does the same for your
> on-call shift — and the flight/"fly" verbs throughout the CLI keep the raven
> conceit.

Munin is a command-line assistant for the signals you check at the start of —
and throughout — an SRE shift. It pulls **GitHub** PRs and review requests,
**Google** Calendar / Gmail / Docs / Drive / Tasks, and **Slack** activity into
one consistently formatted view. Query a single signal ad-hoc, save reusable
**queries** and **filters** and recall them by name, or send Munin on a named
**flight** that fetches a whole set concurrently. Everything runs against your
existing credentials and prints as terminal panels or JSON.

## Getting started

Build (or install) the binary:

```sh
go build -o munin .
# or: go install github.com/codyconfer/munin@latest
```

Bootstrap a config directory (with a sample query, filter, and flight), then run:

```sh
munin install                 # create ~/.munin with defaults
munin fly                     # run the default flight
munin github query            # ad-hoc: your open PRs + review requests
munin fly morning -o json | jq .
```

Munin reuses tools you already have for authentication — the `gh` CLI, `gcloud`
ADC, `$SLACK_TOKEN` — and falls back to `munin login <service>` when they are
absent. See [Authentication](#authentication) for the full resolution order and
required scopes.

## CLI vs TUI

Munin has two front-ends over the same engine:

- **CLI** (default) — you invoke a command and the result prints as
  [viewkit](https://github.com/codyconfer/viewkit) panels (color on a TTY, plain
  text when piped/redirected, or JSON with `-o json`). Interactive prompts stay
  compact and inline.
- **TUI** — `munin tui` opens a full-screen interactive deck: a main menu, live
  flight runs, run **history**, a **directives** browser (view/run/validate/
  create/edit/delete queries, filters, flights, roles), an ad-hoc read-only **audit
  query** screen, and **settings**. `munin tui <flight>` jumps straight to a
  flight; `munin settings` opens just the settings screens.

## How it works

```
signals (fetch) ──▶ filters (regex include/exclude) ──▶ renderer (terminal | json)
```

Each signal normalizes its data into a common item shape, filters narrow the
results, and a renderer prints them. A **query** binds a signal + params +
filters under a name; a **flight** runs a named set of queries concurrently, and
one failing query degrades to an inline error section instead of blanking the
rest. Every run is recorded to a local audit trail (see
[Audit trail](#audit-trail)).

## Queries and filters

A **query** names a signal, its parameters, and the filters to apply; a
**filter** is an ordered set of regex include/exclude rules targeting a field.
Save them once under `~/.munin/queries/` and `~/.munin/filters/` and recall them
by name (`munin query <name>`), or apply filters ad-hoc with `--include` /
`--exclude` / `--filter`.

→ [How to create query and filter config](#create-a-query-and-filter-config)

## Flights

A **flight** ("fly" plays on the raven) is a named, ordered list of saved query
names run concurrently — your whole shift-start sweep in one command. `munin fly
<name>` runs one; bare `munin fly` runs the active role's first flight (or
`default`), or lists what's available.

→ [How to create a flight config](#create-a-flight-config)

## Roles

**Roles** scope what Munin shows so you see only what's relevant to the hat
you're wearing. A role names the flights, queries, and filters it surfaces; while
that role is active, lists and the TUI show only those, and a bare `munin fly`
runs the role's first flight. With no active role, everything is listed. Set the
active role with `--role`, `$MUNIN_ROLE`, or `role:` in config, and inspect your
context with `munin role`.

→ [How to create a role config](#create-a-role-config)

## Create a query and filter config

A **filter set** (`~/.munin/filters/no-bots.yaml`) is an ordered list of regex
rules. An item is kept only if it satisfies every `include` rule and matches no
`exclude` rule; exclusion wins on conflict. Rules target a field: `title`,
`subtitle`, `body` (default), or `meta.<key>`.

```yaml
name: no-bots
rules:
  - field: meta.author
    exclude: "(?i)bot$"
  - field: body
    include: "deploy|incident|blocker"
```

A **query** (`~/.munin/queries/slack-standup.yaml`) bundles a signal, its params,
and the filters to apply — a saved filter set by name, or an inline rule:

```yaml
name: slack-standup
signal: slack
params:
  channel: eng-standup
  limit: "100"
filters:
  - no-bots               # reference a saved filter set
  - exclude: "^:tada:"    # or an inline rule
```

```sh
munin query slack-standup       # run it
munin query show slack-standup  # inspect the definition
munin filter list               # list saved filters
```

Every file may be YAML (`.yaml`/`.yml`) or JSON (`.json`) — the two mix freely.

## Create a flight config

Flights live one-per-file in `~/.munin/flights/` — a named, ordered list of saved
query names:

```yaml
# ~/.munin/flights/triage.yaml
name: triage                 # run by `munin fly triage`
queries: [incidents, my-open-prs]
```

Each entry in `queries:` refers to a file in `~/.munin/queries/`. A query that
fails to build (missing auth, missing channel, …) shows up as an inline error
section rather than aborting the flight.

## Create a role config

Roles live one-per-file in `~/.munin/roles/`; the active role is set in
`config.yaml` (or `--role` / `$MUNIN_ROLE`):

```yaml
# config.yaml — the active role
role: triage
```
```yaml
# ~/.munin/roles/triage.yaml — a role definition
name: triage
flights: [triage]            # bare `munin fly` runs the first of these
queries: [incidents, loki-errors, my-open-prs]
filters: [no-bots]
```

While a role is active, only the flights, queries, and filters it names appear in
lists and the TUI; with no active role, everything is listed. Asking for a
query or flight the active role doesn't name reports why. Validate references and
enums with `munin verify`.

## Configuration

Config lives under `~/.munin/`:

```
~/.munin/
  config.yaml        # global settings + per-signal defaults
  queries/*.yaml     # named, reusable query definitions
  filters/*.yaml     # named, reusable regex filter sets
  flights/*.yaml     # named flights (one per file)
  roles/*.yaml       # role definitions (one per file)
  config.duckdb      # versioned store: source of truth for config + the four directive kinds
  audit.duckdb       # run history (see Audit trail)
  tokens.duckdb      # cached OAuth credentials
```

**Config directory resolution** (highest wins): `--home <dir>` → `$MUNIN_HOME` →
`home:` in `~/.config/munin/settings.yaml` → `~/.munin`. Bootstrap a fresh
directory with `munin install`, archive its files with `munin clean`, or reset it
with `munin nuke`.

**DuckDB is the source of truth.** `config.duckdb` is the store holding the live
state for the config *and* the four directive kinds. On startup Munin
hash-compares each directive's files against DuckDB:

- **match** → load DuckDB (no change).
- **differ** → on a terminal you're prompted: import (overwrite DB), use the file
  this session, use DuckDB, print the file, or delete the file. Non-interactively
  it uses the file and warns — unless `prefer_duckdb: true` in the global
  settings, which always prefers DuckDB.

Nothing is auto-imported; imports happen only when you choose them, and every
import archives the prior version. Inspect current/prior config with `munin
config` / `munin config history`. `--config <file>` uses a config file for **this
session only** (never persisted) — the non-interactive form of "use the file this
session". Any file value can be overridden per-run by a `MUNIN_*` env var (e.g.
`MUNIN_OUTPUT=json`) or a flag; overrides are never persisted.

**Theme** is a global setting: `theme:` in `~/.config/munin/settings.yaml` (or
`$MUNIN_THEME`) selects a viewkit theme (default `retro-dark`); `munin verify`
validates the key.

See [`examples/`](examples/) for copy-paste starters.

### Authentication

Every signal resolves auth as **CLI/ADC → token → OAuth**, so it works whether or
not the usual CLI is installed; when nothing is configured Munin explains the
options instead of failing opaquely.

| Signal | Primary | Fallbacks |
|---|---|---|
| **GitHub** | `gh` CLI (`gh auth login`) | `$GITHUB_TOKEN` / `$GH_TOKEN` → `munin login github` (device flow) |
| **Calendar / Gmail / Docs / Drive / Tasks** | `gcloud` ADC | `munin login google` (browser OAuth) |
| **Slack** | `$SLACK_TOKEN` (xoxp-…) | `munin login slack` (browser OAuth) |

`munin login <github|google|slack>` runs the service's OAuth flow and caches a
token in the DuckDB credential store (`tokens.duckdb`, one row per service);
later runs use the signal's direct API client. Each needs its OAuth app
credentials in config (`*.oauth_client_id` / `_secret`); GitHub uses the device
flow (no secret), Google and Slack use a localhost browser-redirect flow, and
Google tokens auto-refresh.

- **GitHub Enterprise** — set `github.api_url` (e.g.
  `https://ghe.example.com/api/v3`) so the REST fallback targets your instance.
  Device-flow scopes are `github.oauth_scopes` (default `repo read:org`).
- **Google scopes** — a plain `gcloud auth application-default login` does *not*
  grant the read scopes. Munin preflight-checks them and reprints the exact
  `gcloud … --scopes=…` command to run if any are missing.

### Audit trail

Every flight, query, and write is recorded — with timestamps, per-signal timing,
item counts, errors, and the items themselves — into `audit.duckdb`. This is an
audit trail for tracking your workflow and metrics over time, **not a cache**:
results are never read back to answer a live query.

```sh
munin history                 # list recent runs (flights, queries, writes)
munin history show 12         # recall a past run's stored results
```

The file is queryable directly for ad-hoc metrics:

```sql
SELECT name, count(*), sum(count)
FROM runs WHERE kind = 'flight' GROUP BY name;
```

Disable the trail with `audit.enabled: false` (or `MUNIN_AUDIT_ENABLED=false`).
Config versioning is separate — tracked in `config.duckdb` and surfaced via
`munin config history`.

### Encrypted backups

`munin backup` bundles the DuckDB databases (`config`, `audit`, `tokens`) into a
tar and encrypts it with AES-256-GCM. The key is escrowed in a **secret
manager** — `secret_backend: auto` uses the **Bitwarden** (`bw`) or **1Password**
(`op`) CLI when configured, otherwise the **OS keyring**; if none is available it
errors rather than writing an unrecoverable backup.

```sh
munin backup                 # → ./munin-backup-<ts>.tar.enc  (key escrowed)
munin backup --out /secure   # write elsewhere
munin restore <file>         # decrypt + write the databases back into the home
```

`backup.keep: N` retains only the newest N backups (`0` = keep all).
`backup.destination: gdrive` uploads the encrypted file to the app's private
Google Drive `appDataFolder` instead of the current directory. `munin restore`
doesn't depend on opening `config.duckdb`, so it recovers even a corrupted config
DB.

### Command reference

| Command | Description |
|---|---|
| `munin fly [flight]` | Run a named flight (defaults to the role's flight / `default`). |
| `munin query [name]` | Run a saved query by name; no name lists saved queries. |
| `munin query show <name>` | Show a saved query's definition. |
| `munin <signal> query` | Ad-hoc one-off query against a single signal. |
| `munin history` / `history show <id>` | List past runs / recall a run's results. |
| `munin config` / `config history` | Show the active (DB-backed) config / prior versions. |
| `munin backup` / `restore <file>` | Write / restore an encrypted backup of the DuckDB databases. |
| `munin verify [target]` | Validate config/roles/flights/queries (colorized, masks secrets). |
| `munin install` | Create the config directory and initialize it with defaults. |
| `munin clean` | Archive config/query/filter files into `.archive/<timestamp>/`. |
| `munin nuke [--yes]` | Delete the config directory and DuckDB, then reinstall defaults. |
| `munin role` | Show the active role and defined roles. |
| `munin login <service>` | OAuth login for github/google/slack. |
| `munin filter list` / `filter show <name>` | Inspect saved filters. |
| `munin export <directive>` / `import <directive>` | Materialize DuckDB → files / files → DuckDB. |
| `munin tui [flight]` / `munin settings` | Open the interactive TUI / just the settings screens. |

### Common flags

- `--output, -o terminal|json` — output format (JSON is pipeable to `jq`).
- `--home <dir>` — use a different config directory.
- `--config <file>` — use a config file for this session only (not persisted).
- `--role <name>` — activate a role, scoping visible flights/queries/filters.
- `--timeout <dur>` — per-signal fetch timeout (e.g. `45s`, `2m`).
- `--filter <name>` — apply a saved filter set (repeatable).
- `--include <regex>` / `--exclude <regex>` — ad-hoc filters (repeatable).
- `--verbose, -v` — log skipped/errored signals to stderr.

## Data signals

| Signal | Command(s) | Access | Write restrictions |
|---|---|---|---|
| GitHub | `github query` | Read-only | — |
| Google Calendar | `calendar query` (`cal`) | Read-only | — |
| Gmail | `gmail query` | Read-only | — |
| Google Docs | `docs query` | Read-only | — |
| Google Drive | `drive query`, `drive add` | **Read + write** | Creates a file **only** in the configured `drive.dir`; a write to any other folder is rejected *before* the API call. Reads any folder. Uses the full `drive` OAuth scope (folder discovery + create). |
| Google Tasks | `tasks query`, `tasks add` | **Read + write** | Creates a task **only** in the configured `tasks.list`; a write to any other list is rejected *before* the API call. Reads any list. |
| Slack | `slack query --channel <name>` | Read-only | — |

The write restriction is Munin policy enforced in `cmd/tasks.go:resolveWriteTarget`
before the API call — the OAuth token itself grants broader write access, so the
guardrail is Munin's, not the scope's. Writes are recorded in the audit trail as
`write` runs.

```sh
munin tasks add "review the RFC" --due 2026-07-25 --notes "focus on the API"
munin tasks add "oops" --list "Someone Else's List"   # → rejected: read-only
munin drive add "notes.txt" --content "hello" --mime text/plain
munin drive add "x" --dir "Some Other Folder"         # → rejected: read-only
```

## Old

### Built on sisyphus

The reusable machinery behind config, storage, backup, and secrets lives in a
standalone, app-agnostic module, **sisyphus**
(`github.com/codyconfer/sisyphus`) — no munin-specific types. Munin defines its
own config schema and thin adapters over it: `internal/token` (credentials over
`sisyphus/kv`) and `internal/audit` (flights/queries over `sisyphus/journal`).

### Development

```sh
go build ./...
go vet ./...
go test ./...
```

Signal integrations live in `internal/signals/<name>/`, each with offline table
tests driven by recorded fixtures, so the suite needs no network.

Munin depends on the **private** module `github.com/codyconfer/sisyphus`, so
building requires access to that repo and Go configured to fetch it directly
rather than via the public proxy/checksum database:

```sh
go env -w 'GOPRIVATE=github.com/codyconfer/*'
# and git credentials with read access to the codyconfer org
```
