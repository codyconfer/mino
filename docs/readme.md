# Munin

Munin is a command-line assistant for the signals you check at the start of —
and throughout — an SRE shift. It pulls **GitHub** PRs and review requests,
**Google** Calendar / Gmail / Docs / Drive / Tasks, and **Slack** activity into
one consistently formatted view. Query a single signal ad-hoc, save reusable
**queries** and **filters** and recall them by name, or send Munin on a named
**flight** that fetches a whole set concurrently. Everything runs against your
existing credentials and prints as terminal panels or JSON — or through a
**formatter**, a template that turns a run into a report you can paste.

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
| **daemon** *(experimental, `-tags daemon`)* | `munin daemon [flight]` | `make daemon ARGS="work"` | Install the OS service if missing, then start it (idempotent); optional system tray via `daemon.tray` | — | OS logging (journald / launchd / Windows service) |
| **deck** | `munin deck [flight]` (`--tmux` for a multi-pane workspace) | `make run` | Full-screen TUI only; attaches to a running daemon, else starts a **silent** background `serve` that dies with the deck session | TUI | log dir |

`make run` is deck only — it does not leave a serve process behind. `munin deck` is
the interactive front-end (formerly `munin tui`, still accepted as a hidden alias):
a main menu, run **history**, **query** and **flight** builders that build, run,
validate, save, and delete in one view each, a **directives** browser for roles,
**Notes** (notes/tasks/reminders, on those same builders), a **Plugins**
enable/disable screen, accounts, an ad-hoc read-only **audit query** screen, and
**settings**. `munin deck <flight>` jumps straight to a flight; `munin settings`
opens just the settings screens. When the background daemon is installed, its
status strip shows whether it is running.

### tmux workspace (`munin deck --tmux`)

`munin deck --tmux` runs the deck inside a tmux session (named `munin`) so it can
split off auxiliary panes on demand. Outside tmux it creates-or-attaches the
session; inside tmux it uses the current pane. Requires `tmux` on `PATH`.

The deck pane is the **owner**: it runs the background `serve` process, exactly as
it does without `--tmux`. Auxiliary panes are **thin** — they open no database at
all, which keeps them cheap to start rather than being a correctness requirement.
An inbox pane reads the owner's `serve.sock`; a popped-out view reads a JSON
snapshot the owner writes under `<home>/.data/panes/` and re-renders whenever the
owner rewrites it.

Panes are opened by hotkey. The targets ship unbound — bind them under
`keybinds:` in `config.yaml`:

```yaml
keybinds:
  alt+i: pane.inbox   # live event inbox, attached to the owner's stream
  alt+p: pane.pop     # pop the current detail or flight results into a pane
  alt+s: pane.shell   # a plain $SHELL pane
  alt+x: pane.close   # close the most recently opened pane
```

Splits are width-aware: munin splits side-by-side only when both panes would
still clear the deck's 80-column minimum, otherwise it stacks them. Panes are
killed when the deck exits, and they exit on their own within ~2s if the deck is
`SIGKILL`ed.

Flight and query results render as a **git-style tree** — the run is the trunk,
each signal a branch, each item a leaf — in both cli output and the deck. The
trunk is labelled with what you ran: the flight name for `munin fly morning`, the
query name for `munin query my-prs`, the signal for `munin github query`.

## How it works

```
signals (fetch) ──▶ filters (regex include/exclude) ──▶ renderer (terminal | json | formatter)
```

Each signal normalizes its data into a common item shape, filters narrow the
results, and a renderer prints them. A **query** binds a signal + params +
filters under a name; a **flight** runs a named set of queries concurrently, and
one failing query degrades to an inline error section instead of blanking the
rest. Every run is recorded to a local audit trail (see
[Audit trail](#audit-trail)).

## Queries and filters

Every directive document declares its kind with a required `type:` — one of
`query`, `filter`, `flight`, `role`, `formatter` — and may live in any file at
any depth under `~/.munin/`. A `type: query` document is a signal, its parameters, and the
filters to apply. A `type: filter` document is an ordered set of regex
include/exclude rules targeting a field, plus any aliases it exposes; a query
may also carry its own rules inline. `munin install` still creates `queries/`,
`flights/`, and `formatters/`, and saved documents still land there by default, but that is now
a convention — the directory a file sits in no longer decides what it is.

Documents may sit one-per-file or share a file (`---`-separated YAML documents,
a top-level YAML list, or a JSON array), so a filter can live right next to the
query that uses it. Recall a query by name (`munin query <name>`), or apply
filters ad-hoc with `--include` / `--exclude` / `--filter`.

→ [How to create query and filter config](#create-a-query-and-filter-config)

## Flights

A **flight** ("fly" plays on the raven) is a named, ordered list of saved query
names run concurrently — your whole shift-start sweep in one command. `munin fly
<name>` runs one; bare `munin fly` runs the active role's first flight (or
`default`), or lists what's available.

The seeded `default` flight stays a clean `my-open-prs` sweep. Opt-in showcase
flights (also under [`examples/`](../examples/)): **`demo`** is live GitHub
(`signal: github` items with `github.com` URLs); **`notify-smoke`** streams
synthetic toasts (`signal: demo`) for desktop/notify smoke — keep it off the
default path. Try `munin fly demo`, `munin serve notify-smoke`, or
`make run ARGS=demo`.

→ [How to create a flight config](#create-a-flight-config)

## Roles

**Roles** scope what Munin shows so you see only what's relevant to the hat
you're wearing. A role names the flights and queries it surfaces (filters are
queries, so one `queries:` list covers both); while
that role is active, lists and the TUI show only those, and a bare `munin fly`
runs the role's first flight. **Directives → Queries** and **Directives → Flights**
both honour the active role, so the two lists stay consistent with `munin list`.

**No role means everything.** With no active role, every query, filter, and
flight is listed. That is a first-class position in the role ring, not just the
startup default: `alt+]` / `alt+[` in the deck cycle *through* no-role, so you can
step off a role — even the only one you have defined — and see everything again.
Leaving it also runs the role's `exit` hooks and clears its status chips. Set the
active role with `--role`, `$MUNIN_ROLE`, or `role:` in config, and inspect your
context with `munin role` (which prints `(none)` when no role is active).

→ [How to create a role config](#create-a-role-config)

## Plugins & notes

Plugins are **compile-time linked** Go packages — there is no runtime `.so` /
`plugin.Open` loading. Stock munin registers built-in signals plus Notes /
Tasks / Reminders (`munin.ntr`). Team distributions add more in a separate
**overlay** binary.

**Public SDK.** Overlay code imports
[`github.com/codyconfer/munin/plugin`](../plugin/) (and the thin
[`munin/app`](../app/) entrypoint) — not `munin/internal`. Register contributions
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
[`internal/plugin/external/README.md`](../internal/plugin/external/README.md) and
[`examples/README.md`](../examples/README.md).

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
`munin notes ui` — one screen each for **Notes**, **Tasks**, and **Reminders**,
each a list of records with **New** first and one builder/editor behind every
row, on the same scheme as the directive screens (see
[Build notes, tasks, and reminders the same way](#build-notes-tasks-and-reminders-the-same-way)).
Reminders are a **service-only** contribution: the menu entry and create hotkey
appear only while a live `serve`/`daemon` socket is attached (deck's
session-owned silent serve counts).

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
- **`munin daemon [flight]`** — **experimental and off by default**; present only
  in builds made with `-tags daemon` (see
  [the daemon build tag](#the-os-service-daemon-is-experimental-tagsdaemon)).
  Runs Munin as a background **OS service** (systemd
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

A directive file is any `.yaml`/`.yml`/`.json` file under `~/.munin/`, at any
depth: `queries/no-bots.yaml`, `queries/gh/prs.yaml`, and `team/oncall.yaml` all
load. Every document in it needs a `type:`.

A **filter set** (`~/.munin/queries/no-bots.yaml`) is an ordered list of regex
rules and no signal. An item is kept only if it satisfies every `include` rule
and matches no `exclude` rule; exclusion wins on conflict. Rules target a field:
`title`, `subtitle`, `body` (default), or `meta.<key>`. Without a signal there is
nothing to run, so it is only ever referenced by name:

```yaml
name: no-bots
type: filter
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
type: query
title: Standup chatter        # optional display name for flight panels
signal: slack
params:
  channel: eng-standup
  limit: "100"
filters:
  - no-bots               # reference a saved filter set
  - exclude: "^:tada:"    # or an inline rule
```

A query can also carry `rules:` of its own, which apply only to its own results:

```yaml
name: slack-standup
type: query
signal: slack
params:
  channel: eng-standup
rules:
  - field: meta.author
    exclude: "(?i)bot$"
```

Put both in one file with `---` when a filter is only interesting next to its
query. Every document in a multi-document file needs its own `name`:

```yaml
# ~/.munin/queries/standup.yaml
name: no-bots
type: filter
rules:
  - field: meta.author
    exclude: "(?i)bot$"
---
name: slack-standup
type: query
signal: slack
filters: [no-bots]
params:
  channel: eng-standup
```

`type:` is **required** — it is the only thing that decides which kind a
document is, so nothing is inferred from the file's path. The five values are
`query`, `filter`, `flight`, `role`, and `formatter`, and any of them is valid in any file, so
a flight can sit in `queries/` next to the queries it composes and a filter can
sit in `flights/`:

```yaml
# ~/.munin/queries/triage.yaml
name: triage
type: flight
queries: [incidents, loki-errors]
---
name: incidents
type: query
signal: github
params:
  query: "org:acme is:issue label:incident"
```

A document with no `type:` and no directive fields is skipped, so unrelated YAML
can share the config dir. A document that *looks* like a directive — a `signal:`,
`rules:`, `queries:`, `flights:`, `hooks:`, … — but declares no `type:` is a hard
error naming the file and the document's position in it.

`type:` is enforced against the document's shape: `type: query` requires a
`signal:` and keeps that document's `rules:` private to it; `type: filter`
forbids a signal and requires rules, aliases, or keywords; `type: flight`
requires `queries:` and forbids both a signal and filter content; `type: role`
forbids both too; `type: formatter` requires a `template:` and forbids a signal,
filter content, `queries:`, and `flights:`. Two fields are kind-exclusive the
other way round: `template:` may appear only on a formatter, and `formatters:`
only on a role — the singular `formatter:` is the field that attaches one to a
query or flight. Names collide only within a kind, so a query and a flight may
share a name (as `demo` does) but two flights may not, wherever they are defined.

Discovery skips dot-directories (`.data/`, `.plugins/`, `.archive/`), `logs/`,
and the root `config.yaml`/`config.yml`/`config.json` — that name is reserved
only at the root, so a nested `team/config.yaml` is an ordinary directive file.

`name` stays the invocable identifier (`munin query slack-standup`) and the audit
key; `title` is only what you read. When set, `munin fly` heads that query's
results panel with the title instead of the name — rename it freely without
breaking flights, roles, or history.

```sh
munin query slack-standup       # run it
munin query show slack-standup  # inspect the definition
munin list queries              # queries the active role can see
munin list filters              # filters the active role can see
munin list formatters           # formatters the active role can see
munin list --all                # everything, ignoring the active role
munin filter list               # saved filters plus plugin filter engines
```

Every file may be YAML (`.yaml`/`.yml`) or JSON (`.json`) — the two mix freely.

## Build and manage directives without writing YAML

**Directives** is the deck's first menu entry, and it holds one screen per kind of
document, in order: **Flights**, **Queries**, **Roles**, **Reports**, and
**History** (which appears once something has been recorded).

Every one of the first four is the whole surface for those saved documents,
wherever they live: **New** first, then every saved document with a one-line
summary. There are no sub-screens — picking any entry opens one builder view, on a
blank document or on that one, and everything happens there by keybinding:

| key | does |
|---|---|
| `ctrl+r` | run the document as it currently stands |
| `ctrl+t` | validate it and show the findings inline |
| `ctrl+y` | toggle a YAML panel showing exactly what would be saved |
| `ctrl+s` | save (needs a name) |
| `ctrl+g` | copy the last run's text (among the directives, reports only — the **Notes** record editors bind it too) |
| `ctrl+w` | write the last run's text under `<home>/reports` (reports only — the record editors do not bind it) |
| `ctrl+x` | delete, with a confirmation dialog (saved documents only) |
| `tab` | move focus between the form and the results |
| `esc` | back |

Validation runs against what's in the form, not the file on disk, so it catches
problems in edits you haven't saved yet — for a directive, an unknown signal, a
disabled plugin, a filter reference that doesn't resolve, a regex that won't
compile.

Results land in a scrollable panel under the form, not on a separate screen, so
the query that produced them stays in front of you. Focus moves to the results
when a run finishes; `tab` goes back to the form, and `↑/↓`, `pgup/pgdn`, and
`enter` (open the item's link) work on the results while they hold focus.

Both panels are sized to fit the terminal. The form scrolls around the focused
field, marking clipped edges with `⋯`, and the results take the rows their
content needs. When even that will not fit, the panel that does *not* have focus
collapses to a one-line summary; `tab` expands it again. Below roughly 20 rows
the deck's own header and footer leave too little for the builder to lay out
usefully.

Every one of these screens shares the same shell — one document type per kind
behind one editor — so across the directives the keys, the results panel, and the
save/delete behaviour are identical; only the fields differ. A flight takes
an ordered comma-separated list of query names, checked against your saved queries
before it will run or save. **Notes** reuses the same editor through a second,
plugin-local document type, so it inherits the shell but not every key or
behaviour, and its lists reload from the record store rather than being menus
built once from the files on disk.

**History** is the one entry that is not a builder: past runs cannot be edited, so
selecting one opens its recorded results with `r` to refresh and `ctrl+x` to drop
the run (with the same confirmation dialog). Deleting removes the run, the queries
rolled up under it, and their recorded items.

In the query builder `type:` is the first field — `query` or `filter` — and the
rest of the form follows it: `type: filter` drops `signal`, its params, `extra
params`, and `filters` entirely, because a filter document cannot have them.
`type: query` keeps them and requires a signal. Saving always writes the `type:`
line, since a document without one does not load.

Within a query, picking a signal with `←/→` swaps the param fields to match, so
you get `query` and `project` for `github` but `channel` and `limit` for `slack`.
Values you typed into fields that later get hidden are remembered for the
session, so flipping type to compare and back doesn't cost you your input.

Because a run never leaves the view, tuning a regex and re-running is just
`tab`, edit, `ctrl+r` — no retyping and no screen changes. Editing a saved document includes
renaming it: change `name`, save, and the file moves and the old name is dropped
from the store. A **Notes** record is the exception — it is identified by its row
id rather than a name, so there is nothing to rename.

What you save is what the form shows: switch a query to `type: filter` and the
saved document has no `signal:` or `params:`, so it passes the `type: filter`
validation rather than failing to load over a field you could no longer see.

The builder shows one inline rule and the params it knows about, but editing
preserves everything it cannot display: `aliases:`, `keywords:`, rules beyond the
first, and inline (unnamed) entries in `filters:` all survive a round trip.
Params the signal doesn't declare show up in the `extra params` field as `k=v`
pairs rather than being dropped.

The same thing from the shell:

```sh
munin query build --signal github --param query="is:open is:pr author:@me"
munin query build --signal slack --param channel=eng-standup --filter no-bots
munin query build --signal github --param query="is:open" --exclude "(?i)wip" --dry-run
munin query build --signal gmail --param query="is:unread" --save unread-now
```

`--param` and `--filter` repeat. `--include`/`--exclude`/`--field` add one inline
rule. `--dry-run` prints the query document instead of running it, which is the
quickest way to get a starting point for a file you will hand-edit. Without
`--save` nothing is written — the query runs and is forgotten.

Params are per-signal; `munin query build --help` lists the ones munin knows.
Anything else you pass through `--param` (or the builder's `extra params` field)
reaches the signal untouched, which is how plugin-defined params work.

Saving writes the YAML file **and** imports the `directives` row into DuckDB, so a
saved query is immediately runnable by name — no `munin apply` or restart. Because
the store versions every directive file as one row, that import also commits any
other staged edits sitting anywhere under `~/.munin/`.

## Build notes, tasks, and reminders the same way

**Notes** is a plugin contribution on the deck's main menu — a sibling of
**Directives**, not one of its screens — and it holds one screen per kind of
record: **Notes**, **Tasks**, and **Reminders** (the last only while a
`serve`/`daemon` socket is attached). Each screen is the whole surface for that
kind: **New** first, then every record with a one-line summary, and picking either
entry opens one editor — on a blank record or on that one. The `alt+n` / `alt+t` /
`alt+r` hotkeys open the same builders from anywhere on the deck.

A record is not a file, so the keys do slightly different work:

| key | does |
|---|---|
| `ctrl+r` | run the `ntr` signal for the active role — the same fetch a `signal: ntr` query performs. On the **Reminders** editor it runs the reminder job instead, so you see what would fire right now; it never acknowledges anything |
| `ctrl+t` | validate: check that the due parses, and flag a reminder that would fire immediately (already past due) or never (no due, or already done); for a note it just reports the body it would save |
| `ctrl+y` | toggle a YAML panel showing the record |
| `ctrl+s` | save (needs a **title**, not a name) |
| `ctrl+g` | copy the record, with no prior run needed — a record already is text, where a report has to be rendered first |
| `ctrl+x` | delete, with the same confirmation dialog (saved records only) |
| `tab` | move focus between the form and the results |
| `esc` | back |

`ctrl+w` is deliberately unbound here: a record has no file output.

The rest of the deltas against the directive builders:

- Identity is the row **id**, not a name — the editor of a saved record is titled
  `edit note #3` — so there is no rename: changing the title changes the title and
  nothing else. The first `ctrl+s` on a builder turns that same view into the
  editor of the record it just created, so the next `ctrl+s` updates it instead of
  saving a second copy.
- Task `done` is an ordinary editable toggle. Reminder `done` is **read-only in
  the editor**, and one-way on the list: `reminders.done` is the flag the daemon
  reads to keep from notifying you twice, so un-doning is not offered.
- On a list, `enter` edits the row, `x` toggles done (tasks) or marks done
  (reminders, which then drop off the list of open ones), and `r` refreshes. The
  list also refreshes itself when you come back from an editor that saved or
  deleted. It does **not** currently notice a `munin notes add` run by another
  process — press `r` for that.
- A multiline note body cannot take a typed newline in the TUI — a pre-existing
  viewkit form limitation, not a new one. Use `munin notes add <title> <body>` or
  `munin notes update <id> <title> <body>` when the body needs more than one line.

## Create a flight config

A flight is a `type: flight` document — a named, ordered list of saved query
names. New flights land in `~/.munin/flights/`, one per file, but any file under
`~/.munin/` will do:

```yaml
# ~/.munin/flights/triage.yaml
name: triage                 # run by `munin fly triage`
type: flight
queries: [incidents, my-open-prs]
```

Each entry in `queries:` refers to a query document by `name`, not by filename or
directory. A query that fails to build (missing auth, missing channel, …) shows up
as an inline error section rather than aborting the flight.

## Formatters

A **formatter** is a `type: formatter` document holding one Go
[`text/template`](https://pkg.go.dev/text/template) that turns a run's results
into a text report — a standup post, a triage digest, a canned PR or Slack reply.
The rendered text **replaces** the git-tree panels (or the JSON) on stdout, so it
pipes cleanly. New formatters land in `~/.munin/formatters/`, one per file, but
like every directive one may live anywhere under `~/.munin/`:

```yaml
# ~/.munin/formatters/standup.yaml
name: standup
type: formatter
title: Daily Standup         # optional display label
template: |
  ## Standup {{ now | date "2006-01-02" }}
  {{ range .Sections }}
  ### {{ .Title }} ({{ len .Items }})
  {{ range .Items }}- [{{ .Title }}]({{ .URL }}) {{ .Meta.author }}
  {{ end }}{{ end }}
```

Attach one with a `formatter: <name>` field on a query or flight, or choose one
per run with `--formatter`:

```sh
munin fly triage --formatter triage-summary        # ad-hoc
munin fly morning --formatter standup --copy       # render to the clipboard
munin query my-open-prs --formatter pr-nudge --out nudge.md
munin github query -F pr-nudge                     # ad-hoc single-signal query
munin formatter                                    # list what the role can see
munin formatter show standup                       # print the definition
munin formatter render standup morning             # run flight `morning`, render it
munin fly morning -o json | munin formatter render standup --stdin
```

`--formatter` beats the directive's own `formatter:` field. Without `--copy` or
`--out <path>` the report goes to stdout; `--copy` puts it on the clipboard and
`--out` writes it to a file. `render --stdin` reads a `-o json` section array
instead of running anything, so a captured result can be re-rendered.

Within a flight, per-query `formatter:` fields are **ignored** — the flight's
formatter sees the whole result set, so exactly one template renders a run.
`munin serve` and the streaming path ignore formatters entirely: a stream never
has "all the results".

In the deck, formatters and the reports they produce are one screen:
**Directives → Reports**. `render on` is the first field, so the template and the
flight it renders over sit together — `ctrl+r` runs that flight and shows the
rendered report in the results panel, from the draft in the form rather than the
file on disk, so an edit (or a formatter you have not saved yet) renders as typed.
`ctrl+g` copies the rendered text, `ctrl+w` writes it to
`<home>/reports/<name>-<timestamp>.md`, and `ctrl+s`/`ctrl+x` save and delete the
document itself.

### The template data model

The template is executed against one report value:

| Field | Is |
|---|---|
| `.Formatter` | the formatter's name |
| `.Name` | the flight or query the run was rooted at |
| `.Kind` | `"flight"` or `"query"` |
| `.Role` | the active role, empty when none |
| `.Now` | the run timestamp |
| `.Count` | total item count |
| `.Errors` | `[]string`, one entry per section that failed |
| `.Sections` | flat list of sections; each has `.Query .Signal .Title .Items .Meta .Err .Count` |
| `.Queries` | the same data grouped per source query; each has `.Query .Title .Sections .Items .Count` |
| `.Items` | every item, fully flattened; each has `.Kind .Title .Subtitle .Body .URL .Timestamp .Meta .Query .Signal` |

So `range .Queries` gives one block per saved query, `range .Sections` one per
signal section, and `.Items` the whole run as one list to bucket or sort.

A **missing map key renders empty rather than erroring** (`missingkey=zero`),
because `.Meta` is sparse and per-signal — a GitHub item has no `channel`, a
calendar event has no `author`. This is a deliberate difference from query-param
templates, which *do* fail on a missing key. A typo'd struct field
(`.Titel`) still fails the render.

### Template functions

Every function takes the piped value **last**, so `{{ now | date "2006-01-02" }}`
reads in the order it runs:

| Function | Signature |
|---|---|
| `now` | `() time.Time` |
| `date` | `(layout string, t time.Time) string` |
| `rel` | `(t time.Time) string` — `3h ago` |
| `meta` | `(key string, m map[string]string) string` |
| `default` | `(fallback, v string) string` |
| `trim` / `upper` / `lower` / `title` | `(string) string` |
| `join` | `(sep string, xs []string) string` |
| `indent` | `(n int, s string) string` |
| `truncate` | `(n int, s string) string` — rune-safe |
| `count` | `(any) int` |
| `limit` | `(n int, items) []Item` |
| `byMeta` | `(key string, items) []Bucket` — `Bucket{Key, Items}`, sorted by `Key` |
| `withMeta` | `(key, val string, items) []Item` |
| `sortByTime` | `(items) []Item` — newest first |

```
{{ range .Items | sortByTime | limit 5 }}- {{ .Title | truncate 70 }} · {{ rel .Timestamp }}
{{ end }}
{{ range byMeta "repo" .Items }}{{ .Key | default "(none)" }} — {{ len .Items }}
{{ end }}
```

Templates are parsed when directives load, so a template that will not compile is
reported by name up front rather than failing mid-render. Roles scope which
formatters are visible with `formatters:`. Copy-paste starters live in
[`examples/formatters/`](../examples/formatters/).

## Create a role config

A role is a `type: role` document. `munin install` writes them loose at the top of
`~/.munin/`, one per file, but — like every directive — a role may live anywhere
under the home dir. The active role is set in `config.yaml` (or `--role` /
`$MUNIN_ROLE`):

```yaml
# config.yaml — the active role
role: triage
```
```yaml
# ~/.munin/triage.yaml — a role definition
name: triage
type: role
flights: [triage]            # bare `munin fly` runs the first of these
queries: [incidents, loki-errors, my-open-prs, no-bots]
formatters: [triage-summary] # a role listing no formatters sees none
# Optional enter/exit shell hooks (bash on Unix, PowerShell on Windows).
hooks:
  enter:
    bash: |
      echo entering triage
  exit:
    bash: |
      echo leaving triage
```

One `queries:` list covers both queries and filters, since a filter is just
another directive document. `formatters:` is a separate list, and it is opt-in:
a role that names no formatters sees **none**, so add every formatter the role
should be able to run. While a role is active, only the flights,
queries, and formatters it names appear in lists, completion, and the TUI; with
no active role, everything is listed. Asking for a
query or flight the active role doesn't name reports why. Validate references and
enums with `munin verify`. On a role switch, munin runs the previous role’s exit
hooks, then the new role’s enter hooks (see `examples/README.md`).

## Configuration

Config lives under `~/.munin/`:

```
~/.munin/
  config.yaml          # global settings + per-signal defaults — the only mandated name/location
  *.yaml               # directives: roles (what `munin install` writes here)
  queries/*.yaml       # directives: queries and filters (one or many per file)
  flights/*.yaml       # directives: flights (one per file)
  formatters/*.yaml    # directives: formatters — templated reports (one per file)
  team/gh/prs.yaml     # directives may nest arbitrarily; `type:` decides the kind
  icons/*.png          # optional per-state tray/notification icon overrides
  logs/munin.log       # rotating command/serve/deck log sink (cleanable/nukable)
  .data/config.duckdb  # versioned store: source of truth for config + directives
  .data/audit.duckdb   # run history (see Audit trail)
  .data/tokens.duckdb  # cached OAuth credentials
  .data/serve.duckdb   # realtime cursors/watermarks for serve/daemon
  .data/cache.duckdb   # cached signal results + team rosters (see Result cache)
```

`config.yaml` (or `config.yml` / `config.json`) at the root is the one file with a
required name and place. Everything else is discovered by walking the home dir:
any `.yaml`/`.yml`/`.json` file at any depth is read as directives, keyed by its
path relative to the home dir. `queries/`, `flights/`, and `formatters/` are
created by `munin install` and are where new documents are saved, so they stay
the convention, but they carry no meaning of their own. Skipped while walking: dot-directories
(`.data/`, `.plugins/`, `.archive/`), `logs/`, and the root config file — a nested
`team/config.yaml` is just another directive file.

Every DuckDB file lives under `.data/` so the config dir itself stays readable
(and diffable) — the loose files are `config.yaml` and whatever directives you put
beside it.

**Several munin processes can share one home.** DuckDB allows a single read-write
process per file, so munin holds each database only around the work that needs it
and releases it in between. A second process asking for a database that is in use
is handed it within a few milliseconds, which means you can run `munin apply`, or
anything else, in one terminal while a deck is open in another.

A running deck notices this: it polls a revision marker beside the store roughly
once a second and, when another process has written it, reloads its directives in
place. New and changed flights, queries, filters, formatters, and roles appear
without restarting. Changes to `config.yaml` itself — keybinds, theme, timeouts —
still need a restart.

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

**DuckDB is the source of truth.** `.data/config.duckdb` is the store holding the
live state, in two rows: `config` (the root config file) and `directives` (every
directive file, as a map of home-relative path → content, so nesting round-trips
exactly). On startup Munin hash-compares each row against what is on disk:

- **match** → load DuckDB (no change).
- **differ** → the files are treated as **staged changes**. On a terminal you get a
  panel naming the row, what is staged, what is stored, and which files
  changed, with five choices:

| Key | Choice | Effect |
| --- | --- | --- |
| `a` | apply changes | write the staged files to the store |
| `s` | use this session | run with them, leave the store as-is (default on Enter) |
| `i` | ignore staged | run with the stored version instead |
| `d` | discard changes | delete the staged files (asks `y/N` first), keep stored |
| `e` | open in editor | open the config folder with `$EDITOR`, then re-ask |

Non-interactively it uses the staged files and warns — unless `prefer_duckdb: true`
in the global settings, which always prefers DuckDB. `--reconcile
prompt|apply|session|ignore` picks an answer up front, which is what you want in
scripts, hooks, and cron.

Nothing is auto-imported; imports happen only when you choose them, and every
import archives the prior version. **`munin apply [directive]`** (alias `munin
import`) is the non-interactive way to write staged files into the store — it never
prompts, takes `config`, `directives`, or `all`, and defaults to `all`. **`munin
export <directive>`** goes the other way, restoring each directive file at the
home-relative path it was imported from and creating parent directories as needed.
The old `queries`/`flights`/`roles` arguments still work on both, as deprecated
aliases for `directives`. Inspect current/prior config with `munin config` /
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

See [`examples/`](../examples/) for copy-paste starters.

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

### Result cache

Signal results are cached in `.data/cache.duckdb` and reused until they age past
`cache.ttl` (default `60s`), so re-running a flight — or running five queries that
hit the same backend — does not repeat the API calls. Caching happens before
filtering, so one fetch serves several differently-filtered queries.

```yaml
cache:
  ttl: 60s              # "0" disables; MUNIN_CACHE_TTL overrides
  detail_ttl: 5m        # per-item details (see `munin show`); "0" disables
  signals:
    github: 5m          # per-signal override; MUNIN_CACHE_SIGNALS_GITHUB
    calendar: 30s
```

Item details — the body, checks, reviews and comments behind `munin show` and the
deck's details view — are cached separately under `cache.detail_ttl`, because they
are fetched per item rather than per signal. The value resolves local-first:
`cache.detail_ttl` in this home's config, else `detail_cache_ttl` in the global
`settings.yaml`, else `5m`. An explicit `--cache-ttl` overrides both, so
`--cache-ttl 0` disables detail caching along with everything else.

```sh
munin fly work --refresh      # fetch live, then re-warm the cache
munin fly work --no-cache     # read nothing, write nothing
munin fly work --cache-ttl 5m # override the TTL for this run
munin cache stats             # what is cached, and how much is still fresh
munin cache clear github      # drop one signal; no argument drops everything
```

Only signals advertising the `cacheable` capability are cached, so signals reading
local state (`ntr`) always show writes immediately. An explicit `cache.signals`
entry overrides that either way — a duration turns caching on for any signal, `"0"`
turns it off.

If a fetch fails and a cached copy is less than 24h old, munin serves the cached
results and marks the section `(stale <age>)` instead of showing an error; JSON
output carries the same information in the section's `meta` object. Errors are
never cached. The cache is regenerable, so it is excluded from `munin backup`.

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
| `munin fly [flight]` | **cli**: run a named flight (defaults to the role's flight / `default`); `--formatter`/`--copy`/`--out` render it through a formatter. |
| `munin query [name]` | **cli**: run a saved query by name; no name lists saved queries. Takes the same `--formatter`/`--copy`/`--out`. |
| `munin serve [flight]` | **serve**: foreground realtime watcher in the current shell; `--desktop`/`--interval`/`--bell`/`--theme`. |
| `munin daemon [flight]` | **daemon**: install (if needed) then start the OS service; idempotent. |
| `munin daemon install/uninstall/start/stop/restart/status/attach` | Manage the OS service (systemd user unit / launchd agent / Windows service). |
| `munin deck [flight]` | **deck**: open the interactive TUI (daemon if running, else silent session-owned serve that dies with deck). Alias: `tui`. |
| `munin query show <name>` | Show a saved query's definition. |
| `munin formatter [name]` | List the formatters the active role can see; with a name, show its definition. |
| `munin formatter show <name>` / `render <name> [flight]` | Print a formatter's YAML / run a flight and render it (`--stdin` renders a `-o json` section array instead). |
| `munin <signal> query` | Ad-hoc one-off query against a single signal. |
| `munin notes …` / `notes ui` | Notes/Tasks/Reminders CLI and TUI (`ntr` is an alias). |
| `munin version` | Print brand glyph + `MUNIN` + build version (git describe / tag). |
| `munin history` / `history show <id>` | List past runs / recall a run's results. |
| `munin config` / `config history` | Show the active (DB-backed) config / prior versions. |
| `munin backup` / `restore <file>` | Write / restore an encrypted backup of the DuckDB databases. |
| `munin verify [target]` | Validate config/roles/flights/queries/formatters/onboarding (colorized, masks secrets). |
| `munin onboard [--status\|--reset]` | One-time setup gate: GitHub auth + a GitHub-verified GPG signing key. |
| `munin install` | Create the config directory and initialize it with defaults. |
| `munin plugins list` | List compile-time registered plugins and enablement state. |
| `munin plugins enable/disable <id>` | Runtime activation only (`disabled_plugins` in settings). |
| `munin plugins install/uninstall <id>` | Enable/disable plus provision or remove example directive seeds (not dynamic `.so` loading). |
| `munin plugins scaffold <id>` | Generate an overlay-friendly plugin package (public `munin/plugin` SDK). |
| `munin clean` | Archive the config file, `logs/`, and every directive file into `.archive/<timestamp>/`. |
| `munin nuke [--yes]` | Delete the config directory and DuckDB (run `munin install` to recreate defaults). |
| `munin role` | Show the active role and defined roles. |
| `munin login <service>` | OAuth login for github/google/slack. |
| `munin list [queries\|filters\|flights\|roles\|formatters]` | List what the active role can see (`--all` to ignore the role). |
| `munin filter list` / `filter show <name>` | Inspect saved filters and plugin filter engines. |
| `munin query build --signal <name>` | Compose and run an ad-hoc query; `--save <name>` keeps it, `--dry-run` just prints it. |
| `munin export <directive>` | Materialize DuckDB → files (`config`, `directives`, `all`); directives land at their stored relative paths. |
| `munin apply [directive]` | Write staged files → DuckDB (`config`, `directives`, `all`). Never prompts; defaults to `all`. Alias: `munin import`. |
| `munin settings` | Open just the settings screens of the deck. |

### Common flags

- `--output, -o terminal|json` — output format (JSON is pipeable to `jq`).
- `--home <dir>` — use a different config directory.
- `--config <file>` — use a config file for this session only (not persisted).
- `--role <name>` — activate a role, scoping visible flights and queries.
- `--timeout <dur>` — per-signal fetch timeout (e.g. `45s`, `2m`).
- `--reconcile prompt|apply|session|ignore` — answer the staged-config panel up front.
- `--formatter, -F <name>` — render the results through a formatter instead of the
  normal output; beats a `formatter:` field on the query or flight.
- `--copy` — put the formatted report on the clipboard (needs a formatter).
- `--out <path>` — write the formatted report to a file (needs a formatter).
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

### GitHub project boards

The `github` signal has two modes. With `query:` it runs a GitHub **search**
(`is:open is:pr author:@me`). With `project:` it reads a **Projects v2 board** —
one section per board column, which search alone cannot express, because a
column is a project field value and `status:` is not a search qualifier:

```yaml
name: board-in-progress
type: query
signal: github
params:
  project: acme/17              # owner/number, or a project URL
  filter: 'status:"In Progress" repo:acme/escalations is:open -is:pr'
  title: Escalations · In Progress   # optional section heading
  field: Status                      # optional, defaults to Status
  team: acme/platform                # optional, owner/team-slug
```

`filter:` takes the same syntax as a board view's filter bar, so a view's filter
copies straight across: `status:`, `repo:`, `is:` (`open`/`closed`/`merged`/
`draft`/`issue`/`pr`), `assignee:`, `author:`, `label:`, `no:`, `sort:`, plus
bare words as title/body text. Values are comma-OR'd, `-` negates, quote values
containing spaces, and `@me` resolves to the authenticated user. An unsupported
qualifier is a config error rather than a silently-ignored text term.

Everything except `status:`/`no:` runs server-side through the search API scoped
to `project:owner/number`; the field value is read from each result's
`projectItems` and matched locally. This keeps a query to one or two API calls —
paging a whole board would be one call per 100 items. Board columns hold only
issues and pull requests this way; draft (note) cards are not searchable.

Reading a project needs the **`read:project`** scope, which is not in the default
device-flow scope set: `gh auth refresh -s read:project`, or add it to
`github.oauth_scopes` before `munin login github`.

Each item carries the field value in `meta.status`, so filter rules can narrow
further (`field: meta.status`, `field: meta.labels`, `field: meta.assignees`).

### Who owes the next reply

For a board column like *Waiting*, the useful question is not who opened an item
but who spoke last. Every project item carries `meta.last_comment_by` — the
author of the last **human** comment, skipping bots, falling back to the issue
author when there are no comments. Only the last few comments are inspected, so a
thread whose recent history is all bots reports the author.
`meta.last_comment_at` carries when that comment landed (RFC3339, the item's open
time for the author fallback).

Rows render a reply chip next to the author, ending in how long ago that comment
landed: `↩ @cust22 ·3d ago`. Because the chip already dates the thread, it
replaces the row's usual `updatedAt` time rather than sitting beside it.

Set `team: owner/team-slug` and each item also gets `meta.last_comment_team`
(`true` when the last commenter is on that team). The chip then reads green
`↩ @alice ·team ·3d ago` when a teammate replied last and amber
`↩ @cust22 ·3d ago` when the reply came from outside, and a filter rule can keep
only one side:

```yaml
name: escalations-waiting
type: query
signal: github
params:
  project: acme/17
  filter: 'status:Waiting repo:acme/escalations is:open -is:pr'
  team: acme/platform
rules:
  - field: meta.last_comment_team
    include: "false"
```

Team membership costs one extra GraphQL call, cached for 24h in
`.data/serve.duckdb`, and needs the **`read:org`** scope (part of the default
scope set). Without `team:`, `meta.last_comment_team` is absent and the chip
renders dim — so a missing key always means "not configured", never "external".
`meta.last_comment_at` is unaffected by `team:` and present either way.

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
make daemon  ARGS="work"               # install + start the OS service (experimental; sets TAGS=daemon)
make run                               # deck TUI only (silent background serve if needed; dies with deck)
make run ARGS=demo                     # deck on the live-GitHub demo flight
```

Build vars (make variables, not `ARGS`): `RACE=1` (race detector), `TAGS=…` (build
tags), `EMAIL_DOMAIN=…` (adds a domain authorization requirement), and
`ALL_OR_NOTHING_AUTH=1` (compile ordinary cli directives to block rather than warn
when unauthorized). `make package` cross-compiles release binaries.

#### The OS-service daemon is experimental (`TAGS=daemon`)

The OS-service daemon is **off by default**. It is an experimental feature in its
own package, enabled with a build tag:

```sh
make daemon ARGS="work"       # sets TAGS=daemon for you
make build TAGS=daemon        # or opt in explicitly
go build -tags daemon .       # or straight from the toolchain
```

Default builds have no `daemon` command at all — `munin daemon` reports `unknown
command`. The whole feature lives in `github.com/codyconfer/munin/daemon` and is
linked by exactly one file, `experimental_daemon.go`, a blank import behind the
tag. That package's `init()` registers the `daemon` command tree with `cmd`, the
daemon status chip with `statusstrip`, and the `daemon.tray` setting plus the
`daemon` status-bar entry with `views`; nothing in core refers back to it.

What the tag adds: `munin daemon` and its
`install/uninstall/start/stop/restart/status/attach` subcommands, the system
tray, the daemon status chip in `deck`, the `daemon.tray` setting, and the
`kardianos/service` + systray dependencies. Verify the default build carries
none of it:

```sh
go list -deps .              | grep -E 'kardianos|systray'   # empty
go list -deps -tags daemon . | grep -E 'kardianos|systray'   # both present
```

What works either way — the tag changes nothing here: every cli directive,
`deck`, and `munin serve` (the foreground realtime watcher) with its event
socket, desktop notifications, scheduled delivery, the attach notification
inbox, and `deck`'s silent background serve provider. Service-only plugin
contributions also stay available: `plugin.ServiceAttached` keys off the serve
socket rather than the installed service, so NTR reminders show up whenever
something is watching.

Both configurations are checked. `go build ./...` and `go test ./...` compile and
test the daemon package itself in the default build; `make build-experimental`
(part of `make check`) additionally builds and vets the root binary with
`-tags daemon`.

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

Licensed under the GNU Affero General Public License v3.0 — see [LICENSE](../LICENSE).

Munin depends on [sisyphus](https://github.com/codyconfer/sisyphus) and
[viewkit](https://github.com/codyconfer/viewkit), both MIT.
