# Mino

Mino is a command-line assistant for the signals you check at the start of —
and throughout — an SRE shift. It pulls **GitHub** PRs and review requests,
**Google** Calendar / Gmail / Docs / Drive / Tasks, and **Slack** activity into
one consistently formatted view. Query a single signal ad-hoc, save reusable
**queries** and **filters** and recall them by name, or send Mino on a named
**flight** that fetches a whole set concurrently. Everything runs against your
existing credentials and prints as terminal panels or JSON — or through a
**formatter**, a template that turns a run into a report you can paste.

## Contents

**Start here**

- [Getting started](getting-started.md) — build or install the binary, bootstrap
  `~/.mino`, and the fetch → filter → render pipeline behind every run.
- [Operating modes](operating-modes.md) — the four modes over the same engine
  (cli, serve, daemon, deck), their stdio and logging contracts, `make` targets,
  and the `--tmux` workspace.

**Directives** — the YAML documents under `~/.mino/`, each declaring its `type:`

- [Queries and filters](directives/queries-and-filters.md) — a signal plus its
  params, and the ordered regex include/exclude rules that narrow it.
- [Flights](directives/flights.md) — a named, ordered list of saved queries run
  concurrently: your whole shift-start sweep in one command.
- [Roles](directives/roles.md) — scope what Mino shows to the hat you're
  wearing, with optional enter/exit hooks.
- [Formatters](directives/formatters.md) — a Go `text/template` that turns a
  run's results into a report you can paste, plus the data model and functions.
- [Saved DuckDB queries](directives/duckdb.md) — read-only SQL against mino's
  own `audit`, `config`, and `tokens` databases.

**The deck** — the full-screen TUI

- [Building directives without writing YAML](deck/builders.md) — one builder
  view per kind that runs, validates, saves, and deletes in place.
- [Notes, tasks, and reminders](deck/notes.md) — the record editors on that same
  scheme, and the buckets that group records across all three.

**Realtime**

- [serve & daemon](realtime/serve-and-daemon.md) — passive versus active
  signals, the foreground watcher, and the managed OS service.
- [HTTP trigger API](realtime/http-api.md) — the token-guarded `/api/v1`
  endpoints, the SSE event stream, and GitHub device-flow sign-in.
- [Container](realtime/container.md) — the image and compose file, the `MINO_*`
  environment, and what binding off-loopback costs you.

**Configuration**

- [Configuration](configuration/readme.md) — the config directory layout, the
  DuckDB store as source of truth, staged changes, plugin settings, and logs.
- [Onboarding](configuration/onboarding.md) — the GitHub auth + verified signing
  key gate, how each mode enforces it, and the build-time policy switches.
- [Authentication](configuration/authentication.md) — per-signal resolution
  order and scopes, git providers, and service (App / machine-user) auth.
- [Result cache](configuration/cache.md) — `.data/cache.duckdb`, TTLs, and
  serving stale results when a fetch fails.
- [Audit trail](configuration/audit.md) — every flight, query, and write
  recorded to `.data/audit.duckdb`, and how to query it.
- [Encrypted backups](configuration/backups.md) — `mino backup` / `restore` and
  the secret manager that escrows the key.

**Reference**

- [Command reference](reference/commands.md) — every command, and the flags
  common to all of them.
- [Data signals](reference/signals.md) — what each signal reads and writes,
  GitHub project boards, and who owes the next reply.

**Extending**

- [Plugins & notes](plugins.md) — the compile-time plugin model, the public
  `mino/plugin` SDK, and the overlay binary.
- [Development & internals](development.md) — build and test targets, build
  tags, the experimental daemon, and multi-repo `go.work` setups.

Copy-paste starters live in [`examples/`](../examples/).

## License

Copyright (c) 2026 Cody Confer

Licensed under the GNU Affero General Public License v3.0 — see [LICENSE](../LICENSE).

Mino depends on [sisyphus](https://github.com/codyconfer/sisyphus) and
[viewkit](https://github.com/codyconfer/viewkit), both MIT.
