# Munin directive examples (Lane D)

One file per directive — copy into `~/.munin/{queries,filters,flights,roles}/`.
Filenames become the directive name when `name:` is omitted.

| Dir | Purpose |
|---|---|
| `queries/` | Saved signal fetches (`signal:` + optional `params` / `filters`) |
| `filters/` | Named filter rules referenced by queries |
| `flights/` | Ordered lists of query names for `munin fly` / `munin serve` |
| `roles/` | Visibility scopes + optional `contexts:` (ADR-9) |

## Role `contexts:`

On role activation (`--role`, `MUNIN_ROLE`, or config `role:`), munin applies
`contexts:` bindings via each tool’s `ContextProvider`:

```yaml
contexts:
  kubectl: prod
  gcx: myorg.grafana.net
```

See `roles/daily.yaml`, `roles/triage.yaml`, and `roles/ops.yaml`.

## Plugin starters

| Query | Signal | Notes |
|---|---|---|
| `ntr-list` | `ntr` | Notes/tasks; pair with flight `ntr` + `munin serve ntr` for Scheduled reminders |
| `gcx-status` | `gcx` | C-0 offline auth/context status (no live IRM HTTP) |
| `kubectl-context` | `kubectl` | Current kube context |
| `*-context` | gooseai/pi/opencode/ollama | Lane C2 stubs |
| `scaffold-ping` | `scaffold` | ADR-14 template only (not linked into the default binary) |

Flight `plugins` bundles the external stub queries for a quick smoke fly.
