# Queries and filters

Every directive document declares its kind with a required `type:` — one of
`query`, `filter`, `flight`, `role`, `formatter` — and may live in any file at
any depth under `~/.mino/`. A `type: query` document is a signal, its parameters, and the
filters to apply. A `type: filter` document is an ordered set of regex
include/exclude rules targeting a field, plus any aliases it exposes; a query
may also carry its own rules inline. `mino install` still creates `queries/`,
`flights/`, and `formatters/`, and saved documents still land there by default, but that is now
a convention — the directory a file sits in no longer decides what it is.

Documents may sit one-per-file or share a file (`---`-separated YAML documents,
a top-level YAML list, or a JSON array), so a filter can live right next to the
query that uses it. Recall a query by name (`mino query <name>`), or apply
filters ad-hoc with `--include` / `--exclude` / `--filter`.

## Create a query and filter config

A directive file is any `.yaml`/`.yml`/`.json` file under `~/.mino/`, at any
depth: `queries/no-bots.yaml`, `queries/gh/prs.yaml`, and `team/oncall.yaml` all
load. Every document in it needs a `type:`.

A **filter set** (`~/.mino/queries/no-bots.yaml`) is an ordered list of regex
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

A **query** (`~/.mino/queries/slack-standup.yaml`) bundles a signal, its params,
and the filters to apply — a saved filter set by name, or an inline rule. The
examples below use `signal: slack`, which comes from the
[`external/plugins`](../../external/plugins/) overlay; a stock binary has `github`,
`ntr`, and whatever plugins its host registers:

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
# ~/.mino/queries/standup.yaml
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
document is, so nothing is inferred from the file's path. The six values are
`query`, `filter`, `flight`, `role`, `formatter`, and `duckdb`, and any of them is valid in any file, so
a flight can sit in `queries/` next to the queries it composes and a filter can
sit in `flights/`:

```yaml
# ~/.mino/queries/triage.yaml
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
filter content, `queries:`, and `flights:`; `type: duckdb` requires a supported
`database:` and one read-only `sql:` statement. Two fields are kind-exclusive the
other way round: `template:` may appear only on a formatter, and `formatters:`
only on a role — the singular `formatter:` is the field that attaches one to a
query or flight. Names collide only within a kind, so a query and a flight may
share a name (as `demo` does) but two flights may not, wherever they are defined.

Discovery skips dot-directories (`.data/`, `.plugins/`, `.archive/`), `logs/`,
and the root `config.yaml`/`config.yml`/`config.json` — that name is reserved
only at the root, so a nested `team/config.yaml` is an ordinary directive file.

`name` stays the invocable identifier (`mino query slack-standup`) and the audit
key; `title` is only what you read. When set, `mino fly` heads that query's
results panel with the title instead of the name — rename it freely without
breaking flights, roles, or history.

```sh
mino query slack-standup       # run it
mino query show slack-standup  # inspect the definition
mino list queries              # queries the active role can see
mino list filters              # filters the active role can see
mino list formatters           # formatters the active role can see
mino list --all                # everything, ignoring the active role
mino filter list               # saved filters plus plugin filter engines
```

Every file may be YAML (`.yaml`/`.yml`) or JSON (`.json`) — the two mix freely.
