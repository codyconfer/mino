# Munin directive examples (Lane D)

Copy `queries/` and `flights/` into `~/.munin/`, and the loose role `*.yaml`
next to `config.yaml`. A filename becomes the directive name when `name:` is
omitted — but only for single-document files.

| Path | Purpose |
|---|---|
| `queries/` | Saved signal fetches (`signal:` + optional `params` / `filters`) **and** filters (`rules:` / `aliases:` / `keywords:`). A document can be both; add `type: query` or `type: filter` to be explicit. |
| `flights/` | Ordered lists of query names for `munin fly` / `munin serve` |
| `*.yaml` (top level) | Roles: visibility scopes + optional `contexts:` / `hooks:` / `status:` (ADR-9) |

Queries and filters share the `queries/` collection, so they can sit in separate
files or share one — see `queries/no-bots.yaml` (a filter on its own) and
`queries/templated-prs.yaml` (a filter and its query in one `---`-separated
file).

`type:` is the kind discriminator across all four kinds — `query`, `filter`,
`flight`, `role` — so the table above describes defaults, not limits. A document
carrying an explicit `type:` is read as that kind in whichever directory it
sits: see `queries/self-contained-flight.yaml`, a flight declared alongside the
query it composes. Omit `type:` and the directory decides.

## Role `contexts:`, `hooks:`, and `status:`

On role activation (`--role`, `MUNIN_ROLE`, or config `role:`), munin applies
`contexts:` bindings via each tool’s `ContextProvider`:

```yaml
contexts:
  kubectl: prod
  gcx: myorg.example.net
```

Optional `hooks:` run shell scripts when entering or leaving a role. On a role
switch munin runs the previous role’s **exit** hooks, then the new role’s
**enter** hooks, then applies `contexts:`. Bash is preferred on Unix;
PowerShell on Windows. If the preferred script is empty, the other is used when
present. Missing interpreters are warned and skipped (activation continues).
The last entered role is remembered under `~/.munin/.data/active-role` so exit
hooks still run across separate CLI invocations.

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

See `daily.yaml`, `triage.yaml`, and `ops.yaml`.

## Plugin starters

Plugins are **compile-time linked** into the munin binary (ADR-7). Runtime
`enable`/`disable` toggles activation; `munin plugins install <id>` enables and
copies that plugin's example directives into `~/.munin` (config seeds only — no
`.so` loading). `munin plugins uninstall <id>` disables and removes unmodified
seeds.

| Query | Signal | Plugin id | Notes |
|---|---|---|---|
| `ntr-list` | `ntr` | `munin.ntr` | Notes/tasks; reminders are service-only (UI + Scheduled delivery via `munin serve ntr` / daemon) |
| `gcx-status` | `gcx` | `external.gcx` | Overlay-only (`munin-plugins-external`); C-0 offline auth/context |
| `kubectl-context` | `kubectl` | `external.kubectl` | Overlay-only; current kube context |
| `*-context` | gooseai/pi/opencode/ollama | `external.*` | Overlay-only Lane C2 stubs |
| `scaffold-ping` | `scaffold` | `scaffold.example` | ADR-14 template; generate with `munin plugins scaffold` (not linked into the default binary) |

External plugin YAML under `examples/` is reference material for the overlay
binary (`../munin-overlay-template`). Stock `munin` does not register `external.*`.

```sh
munin plugins scaffold team.example --dir ./plugins/example
munin plugins install munin.ntr          # enable + seed queries/ntr-list + flights/ntr
munin notes ui                           # Notes/Tasks TUI; Reminders when serve/daemon attached

# With externals overlay binary:
munin-with-externals plugins install external.kubectl
munin-with-externals plugins uninstall external.kubectl
```

Flight `plugins` bundles the external stub queries for a quick smoke fly (overlay).
Flight `demo` is a live GitHub showcase (`signal: github` queries whose items
carry `github.com` URLs); opt in with `munin fly demo` / `munin serve demo` /
`make run ARGS=demo` — it is not the default flight. Queries `demo` and
`demo-reviews` apply filter `demo` (drops `meta.author` bot matches). Role
`demo` scopes visibility to that flight/queries/filter (`munin --role demo …`).
Synthetic toast spam lives separately in flight `notify-smoke` (`signal: demo`).
