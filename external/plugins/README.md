# external/plugins

The Google (Calendar, Gmail, Docs, Drive, Tasks), Slack, and demo signals, plus
the Lane C / C2 packages (`gcx`, `kubectl`, `ollama`, `argocd`, the shared
`stub` helper, and the `example` overlay sample). They are a **separate Go
module**
(`github.com/codyconfer/mino/external/plugins`) built only against mino's
public SDK — `mino/plugin` for contributions and `mino/cmd` for the CLI
bridge. Stock `mino` does not link them.

## Build the overlay

```sh
cd external/plugins
go build ./...
go test ./...
go run ./overlay calendar query     # the same CLI, plus these signals
```

`overlay/main.go` is the reference host: it hands `plugins.Register` to
`app.Options.RegisterPlugins`, so every contribution below lands before
`cmd.Root()` is built.

## What each plugin contributes

| Plugin | Signal | Contributions |
| --- | --- | --- |
| `calendar` | `calendar` | query + stream, query params, `mino calendar query` (alias `cal`) |
| `gmail` | `gmail` | query, query params, `mino gmail query` |
| `docs` | `docs` | query, query params, `mino docs query` |
| `drive` | `drive` | query, `mino drive query|add`, the `gdrive` backup destination |
| `tasks` | `tasks` | query + stream, `mino tasks query|add` |
| `slack` | `slack` | query + stream, query params, `mino slack query`, the `slack` login provider |
| `google` | — | the shared `google` login provider for the five Google signals |
| `demo` | `demo` | query + stream, the `demo-no-lorem` filter engine |
| `gcx` | `gcx` | offline Grafana Cloud status (sealed token key `gcx`), `declare-incident` / `add-activity` action stubs, seed query |
| `kubectl` | `kubectl` | in-process context + read-only `kubectl config current-context` probe, seed query |
| `ollama` | `ollama` | Lane C2 context stub via `stub`, seed query |
| `argocd` | `argocd` | Lane C2 context stub via `stub`, seed query |
| `example` | `example` | sample team-overlay signal (`overlay.example`) |

`stub/` is the shared helper for new context-tool stubs: `stub.Register(Spec)`
installs the signal, glyph, in-memory context provider, and status chip in one
call. `gcx/SPIKE.md` documents the Grafana Cloud auth matrix and deferrals.

## Team overlay pattern

`overlay/main.go` embeds its own seed tree (`overlay/defaults/`) via
`go:embed` and passes it as `app.Options.Defaults` alongside
`RegisterPlugins: plugins.Register`. A team distribution copies this shape:
private module, private `defaults/` (queries, flights, filters, roles),
private plugin packages, one `main.go`. The artifact is conventionally named
`mino-with-externals`.

## Configuration

Settings live under `plugins.<namespace>.<key>` in `~/.mino/config.yaml`, read
through `plugin.Host.Settings`. Environment overrides follow the usual scheme:
`MINO_PLUGINS_CALENDAR_MAX=20`.

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
    dir: Inbox
  tasks:
    list: My Tasks
  slack:
    token_env: SLACK_TOKEN
    limit: 50
```

`mino login google` and `mino login slack` come back with these plugins
registered — the host resolves login providers from the registry, so the five
Google signal aliases (`mino login calendar`, …) keep working too.

## Example directives

`examples/` holds the directives that reference these signals: `today`
(calendar), `unread-mail` (gmail), `recent-docs` (docs), `slack-standup`
(slack), `notify-smoke` (demo), and the `morning` flight. Copy them into
`~/.mino` when running the overlay.

## Module wiring

`go.mod` carries `replace github.com/codyconfer/mino => ../..` so the overlay
always builds against the tree it ships in. Published consumers pin a mino
version instead; the replace is ignored for non-main modules.
