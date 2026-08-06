# external/plugins

The Google (Calendar, Gmail, Docs, Drive, Tasks), Slack, ArgoCD, and demo
signals, plus the Lane C / C2 packages (`gcx`, `kubectl`, `ollama`, the shared
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
| `slack` | `slack` | query + stream + detail, query params, `mino slack query` / `mino slack show`, a status chip, installable query seeds, the `slack` login provider |
| `google` | — | the shared `google` login provider for the five Google signals |
| `demo` | `demo` | query + stream, the `demo-no-lorem` filter engine |
| `gcx` | `gcx` | live Grafana IRM incidents (`view=incidents`) + offline status (`view=status`), query params, the `gcx` login provider (sealed key `gcx` or `$GCX_TOKEN`), `declare-incident` / `add-activity` actions, `mino gcx query\|declare\|activity\|login`, two seed queries |
| `kubectl` | `kubectl` | query + stream over `kubectl -o json` (unhealthy pods, warning events, node health, rollouts), query params, `mino kubectl query` (alias `k8s`), in-process context provider, two seed queries |
| `ollama` | `ollama` | Lane C2 context stub via `stub`, seed query |
| `argocd` | `argocd` | query + stream + detail against the ArgoCD REST API (sealed token key `argocd`), query params, `mino argocd query\|show`, two seed queries |
| `example` | `example` | sample team-overlay signal (`overlay.example`) |

`stub/` is the shared helper for new context-tool stubs: `stub.Register(Spec)`
installs the signal, glyph, in-memory context provider, and status chip in one
call. `gcx/SPIKE.md` documents the Grafana Cloud auth matrix, the deferrals, and
— in §6 — the design notes for the gcx package, which carries no code comments.
Read §6.1 before touching `gcx/irm.go`: the IRM RPC paths and wire shapes have
not been verified against a real stack. `argocd/SPIKE.md` does the same for
ArgoCD, including what the plugin deliberately does not do.

### argocd

Read-only. It lists Applications with a sync/health rollup, streams state
transitions, and renders a detail panel with the resource breakdown, sync
history, last operation, commit metadata, and conditions. It never sends
`refresh`, which would force a reconcile against your cluster.

Mint a token and grant it read access:

```sh
argocd account generate-token --account mino
```

```text
p, role:mino, applications, get, */*, allow
g, mino, role:mino
```

Supply it through `$ARGOCD_AUTH_TOKEN`, or whatever `plugins.argocd.token_env`
names. A token sealed under the `argocd` credential key wins over the
environment, so a stale `$ARGOCD_AUTH_TOKEN` left behind by the `argocd` CLI
cannot silently repoint mino at another server.

**`app_namespace` and `namespaces` are different things.** ArgoCD has two
namespace concepts and only one is filterable server-side:

- `app_namespace` — where the `Application` resource itself lives, sent to the
  API as `appNamespace`. Apps outside the default `argocd` namespace need it or
  every per-app call 404s.
- `namespaces` — where the workloads land (`spec.destination.namespace`). The
  list API cannot filter on this, so mino applies it client-side after fetching.

Confusing the two produces a filter that silently returns nothing.

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
  argocd:
    server_url: https://argocd.example.com
    token_env: ARGOCD_AUTH_TOKEN
    projects: [platform, storefront]
    only_unhealthy: false
    group_by: none
    max: 50
  kubectl:
    binary: kubectl
    namespace: ""          # empty reads every namespace
    kinds: pods,events,nodes,workloads
    since: 1h
    limit: 25
    restart_threshold: 5
    timeout: 10s
```

`mino login google` and `mino login slack` come back with these plugins
registered — the host resolves login providers from the registry, so the five
Google signal aliases (`mino login calendar`, …) keep working too.

## Example directives

`examples/` holds the directives that reference these signals: `today`
(calendar), `unread-mail` (gmail), `recent-docs` (docs), `notify-smoke` (demo),
and the `morning` flight. Copy them into `~/.mino` when running the overlay.

The Slack directives are installable rather than copy-paste: they are embedded in
`slack/seeds/queries/` and `mino plugins install external.slack` writes
`slack-standup`, `slack-mentions` and `slack-catchup` into `~/.mino/queries`.

## Module wiring

`go.mod` carries `replace github.com/codyconfer/mino => ../..` so the overlay
always builds against the tree it ships in. Published consumers pin a mino
version instead; the replace is ignored for non-main modules.
