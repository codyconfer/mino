# Munin shift assistant

[![CI](https://github.com/codyconfer/munin/actions/workflows/ci.yml/badge.svg)](https://github.com/codyconfer/munin/actions/workflows/ci.yml)

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
munin onboard                 # one-time: GitHub auth + a GitHub-verified GPG key
munin fly                     # run the default flight
munin github query            # ad-hoc: your open PRs + review requests
munin fly morning -o json | jq .
```

On first use Munin guides you through [onboarding](#onboarding) — GitHub auth plus a
GitHub-verified signing key. How it gates depends on the mode: `munin deck` runs the
guided flow; a bare `munin <directive>` prompts to authenticate when you're
unauthenticated, and otherwise warns about any remaining gaps and continues. A
binary compiled with `ALL_OR_NOTHING_AUTH=1` instead blocks ordinary cli
directives while the authenticated account remains unauthorized. Domain locking
can add an authorization requirement, but does not enable blocking by itself.
`login`, `verify`, and `--help` are always available.

Munin reuses tools you already have for authentication — the `gh` CLI, `gcloud`
ADC, `$SLACK_TOKEN` — and falls back to `munin login <service>` when they are
absent. See [Authentication](#authentication) for the full resolution order and
required scopes.

## Operating modes

Munin runs in one of four modes over the same engine. Each has a `munin` command,
a matching `make` target (which builds the binary, then runs it — pass runtime
arguments via `ARGS="…"`), and a fixed stdin/stdout/stderr contract:

| Mode | Command | `make` | What it does | stdout | Logs |
|---|---|---|---|---|---|
| **cli** | `munin <directive>` (`fly`, `query`, `github query`, …) | `make command ARGS="fly work"` | Run a directive and print the result | [viewkit](https://github.com/codyconfer/viewkit) panels (color on a TTY, plain when piped, JSON with `-o json`) | log dir |
| **serve** | `munin serve [flight]` | `make serve ARGS="work"` | Foreground realtime watcher in the current shell (Ctrl-C exits); **no OS service / tray** | live notification stream | shell **and** log dir |
| **daemon** | `munin daemon [flight]` | `make daemon ARGS="work"` | Install the OS service if missing, then start it (idempotent); optional system tray via `daemon.tray` | — | OS logging (journald / launchd / Windows service) |
| **deck** | `munin deck [flight]` | `make run` | Full-screen TUI only; attaches to a running daemon, else starts a **silent** background `serve` that dies with the deck session | TUI | log dir |

`make run` is deck only — it does not leave a serve process behind. `munin deck` is
the interactive front-end (formerly `munin tui`, still accepted as a hidden alias):
a main menu, live flight runs, run **history**, a **directives** browser
(view/run/validate/create/edit/delete queries, filters, flights, roles), **Notes**
(notes/tasks/reminders), a **Plugins** enable/disable screen, accounts, an ad-hoc
read-only **audit query** screen, and **settings**. `munin deck <flight>` jumps
straight to a flight; `munin settings` opens just the settings screens. Its status
strip shows whether the background daemon is installed and running.

Flight and query results render as a **git-style tree** — the flight is the trunk,
each signal a branch, each item a leaf — in both cli output and the deck.

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

The seeded `default` flight stays a clean `my-open-prs` sweep. Opt-in showcase
flights (also under [`examples/`](examples/)): **`demo`** is live GitHub
(`signal: github` items with `github.com` URLs); **`notify-smoke`** streams
synthetic toasts (`signal: demo`) for desktop/notify smoke — keep it off the
default path. Try `munin fly demo`, `munin serve notify-smoke`, or
`make run ARGS=demo`.

→ [How to create a flight config](#create-a-flight-config)

## Roles

**Roles** scope what Munin shows so you see only what's relevant to the hat
you're wearing. A role names the flights, queries, and filters it surfaces; while
that role is active, lists and the TUI show only those, and a bare `munin fly`
runs the role's first flight. With no active role, everything is listed. Set the
active role with `--role`, `$MUNIN_ROLE`, or `role:` in config, and inspect your
context with `munin role`.

→ [How to create a role config](#create-a-role-config)

## Plugins & notes

Plugins are **compile-time linked** Go packages — there is no runtime `.so` /
`plugin.Open` loading. Stock munin registers built-in signals plus Notes /
Tasks / Reminders (`munin.ntr`). Team distributions add more in a separate
**overlay** binary.

**Public SDK.** Overlay code imports
[`github.com/codyconfer/munin/plugin`](plugin/) (and the thin
[`munin/app`](app/) entrypoint) — not `munin/internal`. Register contributions
from `app.Options.RegisterPlugins`, then build that binary. Mark contributions
that belong only to serve/daemon mode with `plugin.WithServiceOnly()` (or
`Descriptor.ServiceOnly`); interactive UI lists hide them unless a live
serve/daemon socket is attached.

**Overlay layout** (sibling checkouts of this repo):

```text
../munin-plugins-external/   # external.* packages (gcx, kubectl, …)
../munin-overlay-template/   # thin binary: RegisterPlugins → externals.Register
```

Stock `munin` does not register `external.*`. Build the overlay with
`cd ../munin-overlay-template && make build`. See
[`internal/plugin/external/README.md`](internal/plugin/external/README.md) and
[`examples/README.md`](examples/README.md).

```sh
munin plugins list
munin plugins enable|disable <id>          # runtime activation (settings)
munin plugins install|uninstall <id>       # enable/disable + example directive seeds
munin plugins scaffold team.example --dir ./plugins/example
munin notes ui                             # Notes/Tasks TUI; Reminders when a serve/daemon is attached (`ntr` is an alias)
```

`install` / `uninstall` provision or remove unmodified example directives into
`~/.munin` — they do not download or dynamically load plugin code. The deck
**Plugins** screen toggles enablement; **Notes** opens the same views as
`munin notes ui`. Reminders are a **service-only** contribution: the menu entry
and create hotkey appear only while a live `serve`/`daemon` socket is attached
(deck's session-owned silent serve counts).

## Realtime: serve & daemon

Signals come in two flavors: **passive** (REST, pulled on demand — the default
`fly`/`query` path) and **active** (a live stream). Two modes consume active
signals — a foreground watcher and a managed OS service:

- **`munin serve [flight]`** runs a long-running watcher in the **current shell**
  (Ctrl-C exits): it opens every active signal in the flight, fans their events
  into one loop, and emits a notification per new item. Flags: `--interval`,
  `--bell`, `--desktop` (OS desktop notifications), `--theme`. It does **not**
  install an OS service or own the system tray — its lifecycle is the shell it
  runs in, and it logs to that shell and the log dir.
- **`munin daemon [flight]`** runs Munin as a background **OS service** (systemd
  user unit on Linux, launchd agent on macOS, Windows service), which logs through
  the OS logging facility. Set `daemon.tray: true` for a system-tray icon on that
  service. Bare `munin daemon` is idempotent: it installs the service if it isn't
  installed (after a confirmation; `--yes`/`--system` to script it), then starts
  it if it isn't running. Manage it explicitly with the subcommands:

```sh
munin daemon                              # install (if needed), then start
munin daemon install [flight] [--system]  # install only
munin daemon start | stop | restart | status | uninstall
munin daemon attach                       # attach a live-notification TUI to the running daemon
```

`munin deck` / `make run` ties these together: attach to a running daemon if one
exists, otherwise start a **silent** session-owned background `serve` (stdio
discarded; logs still go to the log dir). That serve dies with the deck session —
including unexpected death on Unix (lifeline pipe). An installed daemon or a
manually started foreground `serve` is never killed by deck exit.

Only **Slack** is a true websocket (Socket Mode); **GitHub**, **Calendar**, and
**Tasks** have no client websocket, so they're polled at `--interval`; signals with
no realtime support are skipped. Slack Socket Mode needs an app-level `xapp-` token
+ a bot `xoxb-` token (env-var names configurable via `slack.app_token_env` /
`slack.bot_token_env`); without them Slack is skipped.

Desktop/notification icons are embedded (raven, dark + light — pick with `--theme`)
and overridable by dropping `~/.munin/icons/<state>.png`. Realtime defaults live
under `daemon:` in config and are overridden by flags.

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

Roles live one-per-file at the top of `~/.munin/` — every `*.yaml`/`*.yml`/`*.json`
next to `config.yaml` is a role. The active role is set in `config.yaml` (or
`--role` / `$MUNIN_ROLE`):

```yaml
# config.yaml — the active role
role: triage
```
```yaml
# ~/.munin/triage.yaml — a role definition
name: triage
flights: [triage]            # bare `munin fly` runs the first of these
queries: [incidents, loki-errors, my-open-prs]
filters: [no-bots]
# Optional enter/exit shell hooks (bash on Unix, PowerShell on Windows).
hooks:
  enter:
    bash: |
      echo entering triage
  exit:
    bash: |
      echo leaving triage
```

While a role is active, only the flights, queries, and filters it names appear in
lists and the TUI; with no active role, everything is listed. Asking for a
query or flight the active role doesn't name reports why. Validate references and
enums with `munin verify`. On a role switch, munin runs the previous role’s exit
hooks, then the new role’s enter hooks (see `examples/README.md`).

## Configuration

Config lives under `~/.munin/`:

```
~/.munin/
  config.yaml          # global settings + per-signal defaults
  *.yaml               # role definitions (one per file, alongside config.yaml)
  queries/*.yaml       # named, reusable query definitions
  filters/*.yaml       # named, reusable regex filter sets
  flights/*.yaml       # named flights (one per file)
  icons/*.png          # optional per-state tray/notification icon overrides
  logs/munin.log       # rotating command/serve/deck log sink (cleanable/nukable)
  .data/config.duckdb  # versioned store: source of truth for config + the four directive kinds
  .data/audit.duckdb   # run history (see Audit trail)
  .data/tokens.duckdb  # cached OAuth credentials
  .data/serve.duckdb   # realtime cursors/watermarks for serve/daemon
```

Every DuckDB file lives under `.data/` so the config dir itself stays readable
(and diffable) — the only loose files are `config.yaml` and the roles.

**Config directory resolution** (highest wins): `--home`/`--dir` → `$MUNIN_HOME` →
`home:` in `~/.config/munin/settings.yaml` → `~/.munin`. Bootstrap a fresh
directory with `munin install`, archive its files with `munin clean`, or wipe it
with `munin nuke` and run `munin install` again (nuke clears a matching
`settings.yaml` `home:` so install falls back to `~/.munin`).

**Logs.** Diagnostic logs go to a file so they never corrupt command output or the
deck's alt-screen: **cli** and **deck** log to the file only, **serve** logs to both
the shell and the file, and **daemon** logs through the OS logging facility (not the
file). The log dir resolves as `$MUNIN_LOG_DIR` → `log_dir:` in `settings.yaml` →
`<home>/logs`; `munin clean` archives it and `munin nuke` removes it.

**DuckDB is the source of truth.** `.data/config.duckdb` is the store holding the live
state for the config *and* the four directive kinds. On startup Munin
hash-compares each directive's files against DuckDB:

- **match** → load DuckDB (no change).
- **differ** → the files are treated as **staged changes**. On a terminal you get a
  panel naming the directive, what is staged, what is stored, and which files
  changed, with five choices:

| Key | Choice | Effect |
| --- | --- | --- |
| `a` | apply changes | write the staged files to the store |
| `s` | use this session | run with them, leave the store as-is (default on Enter) |
| `i` | ignore staged | run with the stored version instead |
| `d` | discard changes | delete the staged files (asks `y/N` first), keep stored |
| `p` | preview | print the staged content, then re-ask |

Non-interactively it uses the staged files and warns — unless `prefer_duckdb: true`
in the global settings, which always prefers DuckDB. `--reconcile
prompt|apply|session|ignore` picks an answer up front, which is what you want in
scripts, hooks, and cron.

Nothing is auto-imported; imports happen only when you choose them, and every
import archives the prior version. **`munin apply [directive]`** (alias `munin
import`) is the non-interactive way to write staged files into the store — it never
prompts and defaults to `all`. Inspect current/prior config with `munin config` /
`munin config history`. `--config <file>` uses a config file for **this session
only** (never persisted) — the non-interactive form of "use this session". Any file
value can be overridden per-run by a `MUNIN_*` env var (e.g. `MUNIN_OUTPUT=json`) or
a flag; overrides are never persisted.

**Theme** is a global setting: `theme:` in `~/.config/munin/settings.yaml` (or
`$MUNIN_THEME`) selects a viewkit theme (default `retro-dark`); `munin verify`
validates the key.

**Realtime defaults** for `serve`/`daemon` live under `daemon:` in `config.yaml`
(`interval`, `bell`, `desktop`, `tray`, `theme`); command flags override them
where exposed (`tray` is config-only on the installed daemon). Editing config in
the deck (**Settings → Edit config**) merges into the existing file, preserving
sections it doesn't touch.

See [`examples/`](examples/) for copy-paste starters.

### Onboarding

Onboarding requires GitHub authenticated **and** a GPG (or SSH) signing key that git
uses and GitHub has verified. Munin classifies you as **unauthenticated** (no GitHub
auth at all), **unauthorized** (authed but a signing/scope/verification gap), or
**authorized**, and gates each mode differently:

| Mode | unauthenticated | unauthorized | authorized |
|---|---|---|---|
| **cli** | prompt to authenticate, then guided setup; errors block | warn + continue by default; **block** in an `ALL_OR_NOTHING_AUTH` build | run |
| **serve** | warn in logs, run anyway | warn in logs, run anyway | run |
| **daemon** | warn in logs | warn in logs | run |
| **deck** | run the guided onboarding flow, then continue | run the guided flow, then continue | run |

```sh
munin onboard            # guided check + fix instructions, loops until ready
munin onboard --status   # print the checklist without changing anything
munin onboard --reset    # clear the marker (re-onboard on the next run)
```

`onboard` checks four things and, for any gap, prints the exact commands to fix it
(it never generates keys or edits your git config): (1) GitHub auth — `gh` CLI or a
cached token; (2) `git config user.signingkey` is set; (3) that secret key is in
your local GPG keyring; (4) the key's public half is registered on your GitHub
account, so signed commits show **Verified**. `munin verify onboarding` reports the
same checklist. `login`, `verify`, `install`/`clean`/`nuke`, and `--help` skip the
gate entirely. The onboarded state lives in `settings.yaml`.

**Domain-locked builds.** A distribution can be compiled to onboard *only* when the
signing key has a GitHub-verified identity in a specific email domain:

```sh
make package EMAIL_DOMAIN=example.com   # only unlocks for @example.com identities
```

This adds a fifth onboarding check (a verified `@example.com` email on the
registered key) and stamps the domain into the marker, so a binary built for one
domain won't accept an onboarding done by an unrestricted build. Built without the
flag, Munin has no domain restriction. Note this is a distribution-policy control,
not a hardened security boundary — `settings.yaml` is user-writable.

#### All-or-nothing auth (`ALL_OR_NOTHING_AUTH`)

`ALL_OR_NOTHING_AUTH` is a **build-time policy**, not a runtime environment
variable or config setting. Set it while compiling to make ordinary cli
directives return an error instead of continuing when the user is authenticated
but not fully authorized:

```sh
make command ARGS="fly work" ALL_OR_NOTHING_AUTH=1   # cli requires full authorization
make package ALL_OR_NOTHING_AUTH=1                    # build all-or-nothing releases
```

The value is enabled when non-empty; use `1` by convention and omit the variable
to build the default warning-only behavior.

This switch is deliberately narrow:

- It changes only the **cli + unauthorized** case.
- It does not change `serve`, `daemon`, or `deck` behavior.
- It does not change unauthenticated cli behavior: Munin still launches guided
  authentication, and an authentication/onboarding error already blocks.
- It does not block gate-exempt recovery commands such as `login`, `verify`,
  `install`, `clean`, `nuke`, or `--help`.
- In a domain-locked build, failing the domain check counts as unauthorized, but
  blocks cli directives only when `ALL_OR_NOTHING_AUTH` was also enabled.

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
token in the DuckDB credential store (`.data/tokens.duckdb`, one row per service);
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
item counts, errors, and the items themselves — into `.data/audit.duckdb`. This is an
audit trail for tracking your workflow and metrics over time, **not a cache**:
results are never read back to answer a live query.

```sh
munin history                 # list recent runs (flights, queries, writes)
munin history show 12         # recall a past run's stored results
```

The file is queryable directly for ad-hoc metrics:

```sql
SELECT name, kind, count(*) AS runs, coalesce(sum(count), 0) AS items
FROM runs GROUP BY name, kind ORDER BY runs DESC;
```

Disable the trail with `audit.enabled: false` (or `MUNIN_AUDIT_ENABLED=false`).
Config versioning is separate — tracked in `.data/config.duckdb` and surfaced via
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
munin restore <file>         # decrypt + write the databases back into <home>/.data
```

`backup.keep: N` retains only the newest N backups (`0` = keep all).
`backup.destination: gdrive` uploads the encrypted file to the app's private
Google Drive `appDataFolder` instead of the current directory. `munin restore`
doesn't depend on opening `.data/config.duckdb`, so it recovers even a corrupted config
DB.

### Command reference

| Command | Description |
|---|---|
| `munin fly [flight]` | **cli**: run a named flight (defaults to the role's flight / `default`). |
| `munin query [name]` | **cli**: run a saved query by name; no name lists saved queries. |
| `munin serve [flight]` | **serve**: foreground realtime watcher in the current shell; `--desktop`/`--interval`/`--bell`/`--theme`. |
| `munin daemon [flight]` | **daemon**: install (if needed) then start the OS service; idempotent. |
| `munin daemon install/uninstall/start/stop/restart/status/attach` | Manage the OS service (systemd user unit / launchd agent / Windows service). |
| `munin deck [flight]` | **deck**: open the interactive TUI (daemon if running, else silent session-owned serve that dies with deck). Alias: `tui`. |
| `munin query show <name>` | Show a saved query's definition. |
| `munin <signal> query` | Ad-hoc one-off query against a single signal. |
| `munin notes …` / `notes ui` | Notes/Tasks/Reminders CLI and TUI (`ntr` is an alias). |
| `munin version` | Print brand glyph + `MUNIN` + build version (git describe / tag). |
| `munin history` / `history show <id>` | List past runs / recall a run's results. |
| `munin config` / `config history` | Show the active (DB-backed) config / prior versions. |
| `munin backup` / `restore <file>` | Write / restore an encrypted backup of the DuckDB databases. |
| `munin verify [target]` | Validate config/roles/flights/queries/onboarding (colorized, masks secrets). |
| `munin onboard [--status\|--reset]` | One-time setup gate: GitHub auth + a GitHub-verified GPG signing key. |
| `munin install` | Create the config directory and initialize it with defaults. |
| `munin plugins list` | List compile-time registered plugins and enablement state. |
| `munin plugins enable/disable <id>` | Runtime activation only (`disabled_plugins` in settings). |
| `munin plugins install/uninstall <id>` | Enable/disable plus provision or remove example directive seeds (not dynamic `.so` loading). |
| `munin plugins scaffold <id>` | Generate an overlay-friendly plugin package (public `munin/plugin` SDK). |
| `munin clean` | Archive config/query/filter files into `.archive/<timestamp>/`. |
| `munin nuke [--yes]` | Delete the config directory and DuckDB (run `munin install` to recreate defaults). |
| `munin role` | Show the active role and defined roles. |
| `munin login <service>` | OAuth login for github/google/slack. |
| `munin filter list` / `filter show <name>` | Inspect saved filters. |
| `munin export <directive>` | Materialize DuckDB → files. |
| `munin apply [directive]` | Write staged files → DuckDB. Never prompts; defaults to `all`. Alias: `munin import`. |
| `munin settings` | Open just the settings screens of the deck. |

### Common flags

- `--output, -o terminal|json` — output format (JSON is pipeable to `jq`).
- `--home <dir>` — use a different config directory.
- `--config <file>` — use a config file for this session only (not persisted).
- `--role <name>` — activate a role, scoping visible flights/queries/filters.
- `--timeout <dur>` — per-signal fetch timeout (e.g. `45s`, `2m`).
- `--reconcile prompt|apply|session|ignore` — answer the staged-config panel up front.
- `--filter <name>` — apply a saved filter set (repeatable).
- `--include <regex>` / `--exclude <regex>` — ad-hoc filters (repeatable).
- `--verbose, -v` — raise the log level to debug (logs go to the log dir; see [Logs](#configuration)).

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

## Development & internals

### Built on sisyphus

The reusable machinery behind config, storage, backup, and secrets lives in a
standalone, app-agnostic module, **sisyphus**
(`github.com/codyconfer/sisyphus`) — no munin-specific types. Munin defines its
own config schema and thin adapters over it: `internal/token` (credentials over
`sisyphus/kv`) and `internal/audit` (flights/queries over `sisyphus/journal`).

### Development

```sh
make build          # go build ./...
make install        # build munin into GOBIN (or GOPATH/bin), replacing any existing binary
make check          # build + fmt-check + lint + govulncheck + test (CI gate is `make ci`)
make test           # go test ./...
```

Linters (`golangci-lint`, `govulncheck`) live in the nested `tools/` module so
they stay out of the consumer dependency graph. `make lint` / `make fmt` invoke
them via `go tool -modfile=tools/go.mod`.

Run a mode straight from source — each target builds then runs, forwarding `ARGS`:

```sh
make command ARGS="fly work -o json"   # cli
make serve   ARGS="work"               # foreground watcher (current shell)
make daemon  ARGS="work"               # install + start the OS service
make run                               # deck TUI only (silent background serve if needed; dies with deck)
make run ARGS=demo                     # deck on the live-GitHub demo flight
```

Build vars (make variables, not `ARGS`): `RACE=1` (race detector), `TAGS=…` (build
tags, see [`nodaemon`](#daemon-free-builds-nodaemon)), `EMAIL_DOMAIN=…` (adds a
domain authorization requirement), and `ALL_OR_NOTHING_AUTH=1` (compile ordinary
cli directives to block rather than warn when unauthorized). `make package`
cross-compiles release binaries.

#### Daemon-free builds (`nodaemon`)

The `nodaemon` build tag compiles munin without serve/daemon mode:

```sh
make package TAGS=nodaemon    # release binaries with no daemon mode
go build -tags nodaemon .     # or straight from the toolchain
```

What the tag removes: the realtime watcher and its local event socket, the OS
service wiring (systemd/launchd/Windows) and system tray, scheduled delivery,
the attach notification inbox, and the `serve` and `daemon` commands (including
`daemon install/start/stop/status/attach`). `internal/app/daemon` is excluded
wholesale, so none of it is compiled or linked.

What still works: every cli directive, `deck`, and the rest of the CLI. `deck`
no longer starts a background provider and drops the daemon status chip.
Service-only plugin contributions — anything registered with
`plugin.WithServiceOnly()`, such as the NTR reminders view and the
`remind.add` / `remind.done` actions — stay hidden, because `plugin.ServiceAttached`
always reports detached. Note reminder *storage* is unaffected: `munin notes
remind …` and `munin notes catch-up` still work, you just don't get pushed
notifications.

Both configurations are first-class; build and test either with
`go build [-tags nodaemon] ./...` and `go test [-tags nodaemon] ./...`. The
default build is unchanged.

Signal integrations live in `internal/signals/<name>/`, each with offline table
tests driven by recorded fixtures, so the suite needs no network. When no live
provider is already listening, `deck` starts `serve` as a silent session-owned
background process (stdio discarded; logs still go to the log dir; dies with
deck).

Munin's reusable foundations are the public modules
[`github.com/codyconfer/sisyphus`](https://github.com/codyconfer/sisyphus) and
[`github.com/codyconfer/viewkit`](https://github.com/codyconfer/viewkit). CI and
published consumers build against the versions pinned in `go.mod`, using the
standard Go module proxy and checksum database with no private credentials or
`replace` directives.

#### Local multi-repo development (`go.work`)

For simultaneous edits across munin / sisyphus / viewkit (and optionally the
overlay siblings), use an **uncommitted** `go.work` in this repo (gitignored; do
not commit — committed `replace` is rejected). A common local pattern is
`go.work.local` (also gitignored) activated with `GOWORK=go.work.local`:

```sh
# from the munin checkout, with sisyphus and viewkit as siblings:
go work init . ../sisyphus ../viewkit
# or: go work use ../sisyphus ../viewkit
# optional overlay siblings:
#   go work use ../munin-plugins-external ../munin-overlay-template
```

Published consumers and CI ignore `go.work` and resolve the pinned module
versions in `go.mod`. Deck lives in the single `viewkit` module (import path
unchanged: `github.com/codyconfer/viewkit/deck`); `go.mod` excludes retired
nested `viewkit/deck` module versions so the parent wins.

## License

Copyright (c) 2026 Cody Confer

Licensed under the GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

Munin depends on [sisyphus](https://github.com/codyconfer/sisyphus) and
[viewkit](https://github.com/codyconfer/viewkit), both MIT.
