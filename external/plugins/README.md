# external/plugins

The Google (Calendar, Gmail, Docs, Drive, Tasks), Slack, and demo signals. They
are a **separate Go module** (`github.com/codyconfer/munin/external/plugins`)
built only against munin's public SDK — `munin/plugin` for contributions and
`munin/cmd` for the CLI bridge. Stock `munin` does not link them.

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
| `calendar` | `calendar` | query + stream, query params, `munin calendar query` (alias `cal`) |
| `gmail` | `gmail` | query, query params, `munin gmail query` |
| `docs` | `docs` | query, query params, `munin docs query` |
| `drive` | `drive` | query, `munin drive query|add`, the `gdrive` backup destination |
| `tasks` | `tasks` | query + stream, `munin tasks query|add` |
| `slack` | `slack` | query + stream, query params, `munin slack query`, the `slack` login provider |
| `google` | — | the shared `google` login provider for the five Google signals |
| `demo` | `demo` | query + stream, the `demo-no-lorem` filter engine |

## Configuration

Settings live under `plugins.<namespace>.<key>` in `~/.munin/config.yaml`, read
through `plugin.Host.Settings`. Environment overrides follow the usual scheme:
`MUNIN_PLUGINS_CALENDAR_MAX=20`.

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

`munin login google` and `munin login slack` come back with these plugins
registered — the host resolves login providers from the registry, so the five
Google signal aliases (`munin login calendar`, …) keep working too.

## Example directives

`examples/` holds the directives that reference these signals: `today`
(calendar), `unread-mail` (gmail), `recent-docs` (docs), `slack-standup`
(slack), `notify-smoke` (demo), and the `morning` flight. Copy them into
`~/.munin` when running the overlay.

## Module wiring

`go.mod` carries `replace github.com/codyconfer/munin => ../..` so the overlay
always builds against the tree it ships in. Published consumers pin a munin
version instead; the replace is ignored for non-main modules.
