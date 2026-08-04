# Mino directive examples (Lane D)

Copy `queries/`, `flights/`, and `formatters/` into `~/.mino/`, and the loose
role `*.yaml` next to `config.yaml`. A filename becomes the directive name when
`name:` is omitted — but only for single-document files.

| Path | Purpose |
|---|---|
| `queries/` | Saved signal fetches (`signal:` + optional `params` / `filters`) **and** filters (`rules:` / `aliases:` / `keywords:`). A document can be both; add `type: query` or `type: filter` to be explicit. |
| `flights/` | Ordered lists of query names for `mino fly` / `mino serve` |
| `formatters/` | Go `text/template` reports (`template:`) that replace stdout for a run |
| `*.yaml` (top level) | Roles: visibility scopes + optional `contexts:` / `hooks:` / `status:` (ADR-9) |

Queries and filters share the `queries/` collection, so they can sit in separate
files or share one — see `queries/no-bots.yaml` (a filter on its own) and
`queries/templated-prs.yaml` (a filter and its query in one `---`-separated
file).

`type:` is the kind discriminator across all five kinds — `query`, `filter`,
`flight`, `role`, `formatter` — so the table above describes defaults, not
limits. A document carrying an explicit `type:` is read as that kind in
whichever directory it sits: see `queries/self-contained-flight.yaml`, a flight
declared alongside the query it composes. `type:` is **required**: a document
with directive-shaped fields but no `type:` is a hard error naming the file, and
a document with neither is skipped so unrelated YAML can share the directory.

## Formatters

A `type: formatter` document holds one `template:` — a Go `text/template` that
turns a run's results into text and **replaces** the usual panels/JSON on
stdout. Attach one with `formatter: <name>` on a query or flight, or ad-hoc with
`--formatter <name>`; add `--copy` for the clipboard or `--out <path>` for a
file. Roles scope them with `formatters: [names]`, so a role listing none sees
none.

| Formatter | Shows off |
|---|---|
| `formatters/standup.yaml` | `range .Queries` headings, `now \| date`, `.Meta.author`, markdown links |
| `formatters/pr-nudge.yaml` | a canned response for `--copy`: `truncate`, `indent`, a timestamped footer |

```sh
mino formatter                          # list the formatters the role can see
mino formatter show standup             # print the YAML
mino formatter render standup morning   # run flight `morning`, render the report
```

Missing map keys render empty rather than erroring (`missingkey=zero`), because
`.Meta` is sparse per signal. `mino serve` ignores formatters — a stream never
has all the results.

## Role `contexts:`, `hooks:`, and `status:`

On role activation (`--role`, `MINO_ROLE`, or config `role:`), mino applies
`contexts:` bindings via each tool’s `ContextProvider`:

```yaml
contexts:
  kubectl: prod
  gcx: myorg.example.net
```

Optional `hooks:` run shell scripts when entering or leaving a role. On a role
switch mino runs the previous role’s **exit** hooks, then the new role’s
**enter** hooks, then applies `contexts:`. Bash is preferred on Unix;
PowerShell on Windows. If the preferred script is empty, the other is used when
present. Missing interpreters are warned and skipped (activation continues).
The active role is recorded in `~/.mino/.data/state.duckdb` so exit hooks still
run across separate CLI invocations.

```yaml
hooks:
  enter:
    bash: |
      echo entering
    powershell: |
      Write-Host entering
  exit:
    bash: |
      echo leaving
    powershell: |
      Write-Host leaving
```

Optional `status:` blocks run with enter (same bash/PowerShell selection). Each
block names a glyph and a command; the first 20 characters of stdout become a
status-bar chip (glyph + text) while the role is active. Failed commands warn
and skip that chip; they do not fail role activation. Chips clear on exit or
role switch.

```yaml
status:
  - glyph: github
    bash: |
      echo "triage"
    powershell: |
      Write-Output "triage"
```

## Plugin starters

Plugins are **compile-time linked** into the mino binary (ADR-7). Runtime
`enable`/`disable` toggles activation; `mino plugins install <id>` enables and
copies that plugin's example directives into `~/.mino` (config seeds only — no
`.so` loading). `mino plugins uninstall <id>` disables and removes unmodified
seeds.

| Query | Signal | Plugin id | Notes |
|---|---|---|---|
| `ntr-list` | `ntr` | `mino.ntr` | Notes/tasks; reminders are service-only (UI + Scheduled delivery via `mino serve ntr` / daemon) |
| `today` / `unread-mail` / `recent-docs` | calendar/gmail/docs | `external.*` | Moved to [`external/plugins/examples`](../external/plugins/examples/) with their signals |
| `slack-standup` | `slack` | `external.slack` | Moved to `external/plugins/examples` |
| `notify-smoke` | `demo` | `external.demo` | Moved to `external/plugins/examples`; synthetic notify toasts |
| `gcx-status` | `gcx` | `external.gcx` | Overlay-only (`external/plugins`); C-0 offline auth/context |
| `kubectl-context` | `kubectl` | `external.kubectl` | Overlay-only; current kube context |
| `*-context` | gooseai/pi/opencode/ollama/argocd | `external.*` | Overlay-only Lane C2 stubs |
| `scaffold-ping` | `scaffold` | `scaffold.example` | ADR-14 template; generate with `mino plugins scaffold` (not linked into the default binary) |

External plugin YAML under `examples/` is reference material for an overlay
binary. Stock `mino` does not register `external.*`. The Google, Slack, and demo
directives live beside their plugins in
[`external/plugins/examples`](../external/plugins/examples/), built by
`make build-overlay`.

```sh
mino plugins scaffold team.example --dir ./plugins/example
mino plugins install mino.ntr          # enable + seed queries/ntr-list + flights/ntr
mino notes ui                           # Notes/Tasks TUI; Reminders when serve/daemon attached

# With externals overlay binary:
mino-with-externals plugins install external.kubectl
mino-with-externals plugins uninstall external.kubectl
```

Flight `plugins` bundles the external stub queries for a quick smoke fly (overlay).
Synthetic toast spam lives separately in flight `notify-smoke` (`signal: demo`),
which ships with the demo plugin in `external/plugins/examples`.
