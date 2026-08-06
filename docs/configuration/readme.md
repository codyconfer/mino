# Configuration

Config lives under `~/.mino/`:

```
~/.mino/
  config.yaml          # global settings + per-signal defaults — the only mandated name/location
  *.yaml               # directives: roles (what `mino install` writes here)
  queries/*.yaml       # directives: queries and filters (one or many per file)
  flights/*.yaml       # directives: flights (one per file)
  formatters/*.yaml    # directives: formatters — templated reports (one per file)
  duckdb/*.yaml        # directives: saved read-only DuckDB queries
  team/gh/prs.yaml     # directives may nest arbitrarily; `type:` decides the kind
  icons/*.png          # optional per-state tray/notification icon overrides
  logs/mino.log       # rotating command/serve/deck log sink (cleanable/nukable)
  .data/config.duckdb  # versioned store: source of truth for config + directives
  .data/audit.duckdb   # run history (see Audit trail)
  .data/tokens.duckdb  # cached OAuth credentials
  .data/serve.duckdb   # realtime cursors/watermarks for serve/daemon
  .data/cache.duckdb   # cached signal results + team rosters (see Result cache)
```

`config.yaml` (or `config.yml` / `config.json`) at the root is the one file with a
required name and place. Everything else is discovered by walking the home dir:
any `.yaml`/`.yml`/`.json` file at any depth is read as directives, keyed by its
path relative to the home dir. `queries/`, `flights/`, `formatters/`, and `duckdb/` are
created by `mino install` and are where new documents are saved, so they stay
the convention, but they carry no meaning of their own. Skipped while walking: dot-directories
(`.data/`, `.plugins/`, `.archive/`), `logs/`, and the root config file — a nested
`team/config.yaml` is just another directive file.

Every DuckDB file lives under `.data/` so the config dir itself stays readable
(and diffable) — the loose files are `config.yaml` and whatever directives you put
beside it.

**Several mino processes can share one home.** DuckDB allows a single read-write
process per file, so mino holds each database only around the work that needs it
and releases it in between. A second process asking for a database that is in use
is handed it within a few milliseconds, which means you can run `mino apply`, or
anything else, in one terminal while a deck is open in another.

A running deck notices this: it polls a revision marker beside the store roughly
once a second and, when another process has written it, reloads its directives in
place. New and changed flights, queries, filters, formatters, DuckDB queries, and roles appear
without restarting. Changes to `config.yaml` itself — keybinds, theme, timeouts —
still need a restart.

**Config directory resolution** (highest wins): `--home`/`--dir` → `$MINO_HOME` →
`home:` in `~/.config/mino/settings.yaml` → `~/.mino`. Bootstrap a fresh
directory with `mino install`, archive its files with `mino clean`, or wipe it
with `mino nuke` and run `mino install` again (nuke clears a matching
`settings.yaml` `home:` so install falls back to `~/.mino`).

**Logs.** Diagnostic logs go to a file so they never corrupt command output or the
deck's alt-screen: **cli** and **deck** log to the file only, **serve** logs to both
the shell and the file, and **daemon** logs through the OS logging facility (not the
file). The log dir resolves as `$MINO_LOG_DIR` → `log_dir:` in `settings.yaml` →
`<home>/logs`; `mino clean` archives it and `mino nuke` removes it.

**DuckDB is the source of truth.** `.data/config.duckdb` is the store holding the
live state, in two rows: `config` (the root config file) and `directives` (every
directive file, as a map of home-relative path → content, so nesting round-trips
exactly). On startup Mino hash-compares each row against what is on disk:

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
import archives the prior version. **`mino apply [directive]`** (alias `mino
import`) is the non-interactive way to write staged files into the store — it never
prompts, takes `config`, `directives`, or `all`, and defaults to `all`. **`mino
export <directive>`** goes the other way, restoring each directive file at the
home-relative path it was imported from and creating parent directories as needed.
The old `queries`/`flights`/`roles` arguments still work on both, as deprecated
aliases for `directives`. Inspect current/prior config with `mino config` /
`mino config history`. `--config <file>` uses a config file for **this session
only** (never persisted) — the non-interactive form of "use this session". Any file
value can be overridden per-run by a `MINO_*` env var (e.g. `MINO_OUTPUT=json`) or
a flag; overrides are never persisted.

**Theme** is a global setting: `theme:` in `~/.config/mino/settings.yaml` (or
`$MINO_THEME`) selects a viewkit theme (default `retro-dark`); `mino verify`
validates the key.

**Plugin settings** live under `plugins:`, namespaced per plugin — stock mino's own
knobs (`github:`, `gitlab:`, `gitea:`, `cache:`, `daemon:`, `backup:`, `audit:`) stay at the top level, and
everything a plugin reads goes under `plugins.<namespace>.<key>`, reached through
`plugin.Host.Settings`:

```yaml
plugins:
  google:
    oauth_client_id: xxxx.apps.googleusercontent.com
    oauth_client_secret: xxxx
  calendar:
    calendar_id: primary
    window: 24h
    max: 50
  drive:
    dir: Inbox              # the single writable folder
  tasks:
    list: My Tasks          # the single writable list
  slack:
    token_env: SLACK_TOKEN
    limit: 50
```

Leaves take `MINO_*` env overrides like any other key
(`MINO_PLUGINS_CALENDAR_MAX=20`). Signals that used to read a top-level section —
`calendar:`, `gmail:`, `docs:`, `drive:`, `tasks:`, `slack:`, `google:` — now read
these namespaces instead, because they ship in
[`external/plugins`](../../external/plugins/) rather than the stock binary; move those
sections under `plugins:` when you build the overlay.

**Realtime defaults** for `serve`/`daemon` live under `daemon:` in `config.yaml`
(`interval`, `bell`, `desktop`, `tray`, `theme`, plus `http.enabled`, `http.host`, `http.port`,
`http.token` and `http.max_concurrent` for the
[HTTP trigger API](../realtime/http-api.md) — leave `http.token` unset and mino generates
one in `.data/http.token`). The deck also uses `interval`
to refresh home-flight, flight-result, and detail views while they show an
in-progress spinner; command flags override the serve/daemon value
where exposed (`tray` is config-only on the installed daemon). Editing config in
the deck (**Settings → Config**) merges into the existing file, preserving
sections it doesn't touch.

See [`examples/`](../../examples/) for copy-paste starters.
