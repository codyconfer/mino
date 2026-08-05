# Roles

**Roles** scope what Mino shows so you see only what's relevant to the hat
you're wearing. A role names the flights and queries it surfaces (filters are
queries, so one `queries:` list covers both); while
that role is active, lists and the TUI show only those, and a bare `mino fly`
runs the role's first flight. **Directives → Queries** and **Directives → Flights**
both honour the active role, so the two lists stay consistent with `mino list`.

**No role means everything.** With no active role, every query, filter, and
flight is listed. That is a first-class position in the role ring, not just the
startup default: `alt+]` / `alt+[` in the deck cycle *through* no-role, so you can
step off a role — even the only one you have defined — and see everything again.
Leaving it also runs the role's `exit` hooks and clears its status chips.

**Where the active role lives.** `mino role use <name>` and `mino role clear`
record it in `.data/state.duckdb`; they never edit your config file. Four
sources resolve it, highest first:

| Source | Scope | Hooks | Persists |
| --- | --- | --- | --- |
| `--role <name>` | one invocation | no | no |
| `$MINO_ROLE` | one invocation | no | no |
| the recorded active role | until changed | on change | yes |
| `role:` in config | seeds the first run | on first apply | seeds the record |

So `role:` is a **default**, not a live switch: editing it once a role has been
activated (or explicitly cleared) changes nothing. Inspect your context with
`mino role`, which prints `(none)` when no role is active.

## Create a role config

A role is a `type: role` document. `mino install` writes them loose at the top of
`~/.mino/`, one per file, but — like every directive — a role may live anywhere
under the home dir. Activate one with `mino role use triage`; `role:` in
`config.yaml` only seeds the first run (see above):

```yaml
# config.yaml — the default role, applied until one is activated
role: triage
```
```yaml
# ~/.mino/triage.yaml — a role definition
name: triage
type: role
flights: [triage]            # bare `mino fly` runs the first of these
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
enums with `mino verify`. On a role switch, mino runs the previous role’s exit
hooks, then the new role’s enter hooks (see `examples/README.md`).
