# Data signals

| Signal | Ships in | Command(s) | Access | Write restrictions |
|---|---|---|---|---|
| GitHub | stock | `github query` | Read-only | — |
| Notes / Tasks / Reminders / Buckets | stock | `mino notes` | **Read + write** | Local DuckDB store under `<home>/.data`: `notes`, `tasks`, `reminders`, plus `buckets` and `bucket_members`. |
| Google Calendar | overlay | `calendar query` (`cal`) | Read-only | — |
| Gmail | overlay | `gmail query` | Read-only | — |
| Google Docs | overlay | `docs query` | Read-only | — |
| Google Drive | overlay | `drive query`, `drive add` | **Read + write** | Creates a file **only** in the configured `plugins.drive.dir`; a write to any other folder is rejected *before* the API call. Reads any folder. Uses the full `drive` OAuth scope (folder discovery + create). |
| Google Tasks | overlay | `tasks query`, `tasks add` | **Read + write** | Creates a task **only** in the configured `plugins.tasks.list`; a write to any other list is rejected *before* the API call. Reads any list. |
| Slack | overlay | `slack query --channel <name>` | Read-only | — |
| ArgoCD | overlay | `argocd query`, `argocd show <url>` | Read-only | Never sends `refresh`, which would force a reconcile against the cluster. |
| Demo | overlay | `query demo` | Read-only | Synthetic items for smoke-testing notifications. |

The write restriction is Mino policy enforced in `cmd/tasks.go:resolveWriteTarget`
before the API call — the OAuth token itself grants broader write access, so the
guardrail is Mino's, not the scope's. Writes are recorded in the audit trail as
`write` runs.

```sh
mino tasks add "review the RFC" --due 2026-07-25 --notes "focus on the API"
mino tasks add "oops" --list "Someone Else's List"   # → rejected: read-only
mino drive add "notes.txt" --content "hello" --mime text/plain
mino drive add "x" --dir "Some Other Folder"         # → rejected: read-only
```

## GitHub project boards

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
`github.oauth_scopes` before `mino login github`.

Each item carries the field value in `meta.status`, so filter rules can narrow
further (`field: meta.status`, `field: meta.labels`, `field: meta.assignees`).

## ArgoCD

Read-only, against the ArgoCD REST API. One item per Application, ordered
worst-first so a broken deploy sits at the top of the panel.

```yaml
name: argocd-unhealthy
type: query
signal: argocd
params:
  only_unhealthy: "true"
  project: platform
  max: "25"
```

Minimum config is the server URL:

```yaml
plugins:
  argocd:
    server_url: https://argocd.example.com
```

`server_url` must be https — mino refuses to send the API token over plain
http, the same rule the GitHub signal applies to `github.api_url`. For a private
or self-signed CA, point `plugins.argocd.ca_file` at the PEM bundle rather than
disabling verification.

Mint a token and grant it read access:

```sh
argocd account generate-token --account mino
export ARGOCD_AUTH_TOKEN=...
```

```text
p, role:mino, applications, get, */*, allow
g, mino, role:mino
```

A token sealed under the `argocd` credential key takes precedence over the
environment variable, so a stale `$ARGOCD_AUTH_TOKEN` from the `argocd` CLI
cannot silently repoint mino at a different server. Change which variable is
consulted with `plugins.argocd.token_env`.

**The plugin never sends `refresh`.** `GET /applications?refresh=hard` forces a
reconcile, which is a write-ish side effect against the cluster; a test asserts
the parameter's absence.

Each item carries the sync/health rollup in `meta.state` (`synced`,
`out of sync`, `progressing`, `degraded`, `missing`, `failed`, `suspended`,
`unknown`) plus `meta.sync`, `meta.health`, `meta.phase`, `meta.revision`,
`meta.project`, `meta.cluster`, and `meta.namespace` — all available to filter
rules and formatters. An application mid-sync sets `meta.in_progress`, which
drives the row spinner and the detail view's live re-poll.

`mino argocd show <application-url>` opens the detail panel: resource breakdown
with per-resource health, the last five sync-history entries, the last
operation with any failed resources, commit metadata, and the ArgoCD conditions
that explain a stuck application.

Two namespace settings exist and they are not interchangeable. `app_namespace`
is where the `Application` resource lives and is sent to the API as
`appNamespace`; apps outside the default `argocd` namespace need it. `namespaces`
filters on `spec.destination.namespace`, where the workloads land — the list API
cannot filter on that, so mino applies it client-side after fetching.

## Who owes the next reply

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
