# Command reference

| Command | Description |
|---|---|
| `mino fly [flight]` | **cli**: run a named flight (defaults to the role's flight / `default`); `--formatter`/`--copy`/`--out` render it through a formatter. |
| `mino query [name]` | **cli**: run a saved query by name; no name lists saved queries. Takes the same `--formatter`/`--copy`/`--out`. |
| `mino serve [flight]` | **serve**: foreground realtime watcher in the current shell; `--desktop`/`--interval`/`--bell`/`--theme`/`--http`/`--http-port`. |
| `mino daemon [flight]` | **daemon**: install (if needed) then start the OS service; idempotent. |
| `mino daemon install/uninstall/start/stop/restart/status/attach` | Manage the OS service (systemd user unit / launchd agent / Windows service). |
| `mino deck [flight]` | **deck**: open the interactive TUI (daemon if running, else silent session-owned serve that dies with deck). Alias: `tui`. |
| `mino query show <name>` | Show a saved query's definition. |
| `mino formatter [name]` | List the formatters the active role can see; with a name, show its definition. |
| `mino formatter show <name>` / `render <name> [flight]` | Print a formatter's YAML / run a flight and render it (`--stdin` renders a `-o json` section array instead). |
| `mino <signal> query` | Ad-hoc one-off query against a single signal. |
| `mino notes …` / `notes ui` | Notes/Tasks/Reminders CLI and TUI (`ntr` is an alias). |
| `mino version` | Print brand glyph + `MINO` + build version (git describe / tag). |
| `mino history` / `history show <id>` | List past runs / recall a run's results. |
| `mino config` / `config history` | Show the active (DB-backed) config / prior versions. |
| `mino backup` / `restore <file>` | Write / restore an encrypted backup of the DuckDB databases. |
| `mino verify [target]` | Validate config/roles/flights/queries/formatters/onboarding (colorized, masks secrets). |
| `mino onboard [--status\|--reset]` | One-time setup gate: GitHub auth + a GitHub-verified GPG signing key. |
| `mino install` | Create the config directory and initialize it with defaults. |
| `mino plugins list` | List compile-time registered plugins and enablement state. |
| `mino plugins enable/disable <id>` | Runtime activation only (`disabled_plugins` in settings). |
| `mino plugins install/uninstall <id>` | Enable/disable plus provision or remove example directive seeds (not dynamic `.so` loading). |
| `mino plugins scaffold <id>` | Generate an overlay-friendly plugin package (public `mino/plugin` SDK). |
| `mino clean` | Archive the config file, `logs/`, and every directive file into `.archive/<timestamp>/`. |
| `mino nuke [--yes]` | Delete the config directory and DuckDB (run `mino install` to recreate defaults). |
| `mino role` | Show the active role and defined roles. |
| `mino login <service>` | OAuth login for github, plus any provider a plugin contributes (google/slack with the overlay). |
| `mino list [queries\|filters\|flights\|roles\|formatters]` | List what the active role can see (`--all` to ignore the role). |
| `mino filter list` / `filter show <name>` | Inspect saved filters and plugin filter engines. |
| `mino query build --signal <name>` | Compose and run an ad-hoc query; `--save <name>` keeps it, `--dry-run` just prints it. |
| `mino export <directive>` | Materialize DuckDB → files (`config`, `directives`, `all`); directives land at their stored relative paths. |
| `mino apply [directive]` | Write staged files → DuckDB (`config`, `directives`, `all`). Never prompts; defaults to `all`. Alias: `mino import`. |
| `mino settings` | Open just the settings screens of the deck. |

## Common flags

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
- `--verbose, -v` — raise the log level to debug (logs go to the log dir; see [Logs](../configuration/readme.md)).
