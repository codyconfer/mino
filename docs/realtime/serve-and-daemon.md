# Realtime: serve & daemon

Signals come in two flavors: **passive** (REST, pulled on demand — the default
`fly`/`query` path) and **active** (a live stream). Two modes consume active
signals — a foreground watcher and a managed OS service:

- **`mino serve [flight]`** runs a long-running watcher in the **current shell**
  (Ctrl-C exits): it opens every active signal in the flight, fans their events
  into one loop, and emits a notification per new item. Flags: `--interval`,
  `--bell`, `--desktop` (OS desktop notifications), `--theme`, `--http`,
  `--http-port` (see [HTTP trigger API](http-api.md)). It does **not**
  install an OS service or own the system tray — its lifecycle is the shell it
  runs in, and it logs to that shell and the log dir.
- **`mino daemon [flight]`** — **experimental and off by default**; present only
  in builds made with `-tags daemon` (see
  [the daemon build tag](../development.md#the-os-service-daemon-is-experimental-tagsdaemon)).
  Runs Mino as a background **OS service** (systemd
  user unit on Linux, launchd agent on macOS, Windows service), which logs through
  the OS logging facility. Set `daemon.tray: true` for a system-tray icon on that
  service. Bare `mino daemon` is idempotent: it installs the service if it isn't
  installed (after a confirmation; `--yes`/`--system` to script it), then starts
  it if it isn't running. Manage it explicitly with the subcommands:

```sh
mino daemon                              # install (if needed), then start
mino daemon install [flight] [--system]  # install only
mino daemon start | stop | restart | status | uninstall
mino daemon attach                       # attach a live-notification TUI to the running daemon
```

`mino deck` / `make run` ties these together: attach to a running daemon if one
exists, otherwise start a **silent** session-owned background `serve` (stdio
discarded; logs still go to the log dir). That serve dies with the deck session —
including unexpected death on Unix (lifeline pipe). An installed daemon or a
manually started foreground `serve` is never killed by deck exit.

Only **Slack** is a true websocket (Socket Mode); **GitHub**, **Calendar**, and
**Tasks** have no client websocket, so they're polled at `--interval`; signals with
no realtime support are skipped. Slack and Calendar/Tasks come from the
[`external/plugins`](../../external/plugins/) overlay, so a stock binary polls GitHub
and nothing else. Slack Socket Mode needs an app-level `xapp-` token + a bot
`xoxb-` token (env-var names configurable via `plugins.slack.app_token_env` /
`plugins.slack.bot_token_env`); without them Slack is skipped.

Desktop/notification icons are embedded (bird, dark + light — pick with `--theme`)
and overridable by dropping `~/.mino/icons/<state>.png`. Realtime defaults live
under `daemon:` in config (including `daemon.http` for the
[HTTP trigger API](http-api.md)) and are overridden by flags.
