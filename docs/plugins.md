# Plugins & notes

Plugins are **compile-time linked** Go packages — there is no runtime `.so` /
`plugin.Open` loading. Stock mino registers **GitHub** plus Notes / Tasks /
Reminders (`mino.ntr`) — nothing else. Google (Calendar / Gmail / Docs / Drive /
Tasks), Slack, and the demo signal are plugins in this repo's
[`external/plugins`](../external/plugins/) module; team distributions add more in
their own **overlay** binary.

**Public SDK.** Overlay code imports
[`github.com/codyconfer/mino/plugin`](../plugin/) (and the thin
[`mino/app`](../app/) entrypoint) — not `mino/internal`. Register contributions
from `app.Options.RegisterPlugins`, then build that binary. Mark contributions
that belong only to serve/daemon mode with `plugin.WithServiceOnly()` (or
`Descriptor.ServiceOnly`); interactive UI lists hide them unless a live
serve/daemon socket is attached.

**Overlay layout.** In-repo, `external/plugins/` is its own Go module built only
against the public SDK, with `overlay/main.go` as a reference host:

```text
external/plugins/            # calendar, gmail, docs, drive, tasks, slack, demo, google login,
                             # gcx, kubectl, ollama, argocd, stub, example
external/plugins/overlay/    # thin binary: RegisterPlugins → plugins.Register,
                             # embedded overlay/defaults/ seed tree
```

```sh
cd external/plugins && go build ./... && go run ./overlay calendar query
# or from the repo root: make build-overlay · make test-overlay
```

Stock `mino` registers none of these. Beyond signals, a plugin can contribute a
**login provider** (`plugin.RegisterLoginProvider`, so `mino login google` works
again), **query params** (`plugin.RegisterQueryParams`), a **backup destination**
(`plugin.RegisterBackupDestination`, which is where `backup.destination: gdrive`
comes from), CLI **commands** (`cmd.RegisterCommand` + `cmd.SignalCmd`), filter
engines, views, themes, and status chips. Each reads its own settings from
`plugins.<namespace>.<key>` in `config.yaml` through `plugin.Host.Settings`. See
[`external/plugins/README.md`](../external/plugins/README.md) and
[`examples/README.md`](../examples/README.md).

```sh
mino plugins list
mino plugins enable|disable <id>          # runtime activation (settings)
mino plugins install|uninstall <id>       # enable/disable + example directive seeds
mino plugins scaffold team.example --dir ./plugins/example
mino notes ui                             # Notes/Tasks TUI; Reminders when a serve/daemon is attached (`ntr` is an alias)
```

`install` / `uninstall` provision or remove unmodified example directives into
`~/.mino` — they do not download or dynamically load plugin code. The deck
**Plugins** screen toggles enablement; **Notes** opens the same views as
`mino notes ui` — one screen each for **Notes**, **Tasks**, and **Reminders**,
each a list of records with **New** first and one builder/editor behind every
row, on the same scheme as the directive screens (see
[Build notes, tasks, and reminders the same way](deck/notes.md)).
Reminders are a **service-only** contribution: the menu entry and create hotkey
appear only while a live `serve`/`daemon` socket is attached (deck's
session-owned silent serve counts).
