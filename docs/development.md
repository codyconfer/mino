# Development & internals

## Built on sisyphus

The reusable machinery behind config, storage, backup, and secrets lives in a
standalone, app-agnostic module, **sisyphus**
(`github.com/codyconfer/sisyphus`) — no mino-specific types. Mino defines its
own config schema and thin adapters over it: `internal/token` (credentials over
`sisyphus/kv`) and `internal/audit` (flights/queries over `sisyphus/journal`).

## Development

```sh
make build          # go build ./...
make install        # build mino into GOBIN (or GOPATH/bin), replacing any existing binary
make check          # build + fmt-check + lint + govulncheck + test (CI gate is `make ci`)
make test           # go test ./...
```

Linters (`golangci-lint`, `govulncheck`) live in the nested `tools/` module so
they stay out of the consumer dependency graph. `make lint` / `make fmt` invoke
them via `go tool -modfile=tools/go.mod`.

Run a mode straight from source — each target builds then runs, forwarding `ARGS`:

```sh
make command ARGS="fly work -o json"   # cli
make serve   ARGS="work"               # foreground watcher (current shell)
make daemon  ARGS="work"               # install + start the OS service (experimental; sets TAGS=daemon)
make run                               # deck TUI only (silent background serve if needed; dies with deck)
```

Build vars (make variables, not `ARGS`): `RACE=1` (race detector), `TAGS=…` (build
tags), `EMAIL_DOMAIN=…` (adds a domain authorization requirement), and
`ALL_OR_NOTHING_AUTH=1` (compile ordinary cli directives to block rather than warn
when unauthorized). `make package` cross-compiles release binaries.

## The OS-service daemon is experimental (`TAGS=daemon`)

The OS-service daemon is **off by default**. It is an experimental feature in its
own package, enabled with a build tag:

```sh
make daemon ARGS="work"       # sets TAGS=daemon for you
make build TAGS=daemon        # or opt in explicitly
go build -tags daemon .       # or straight from the toolchain
```

Default builds have no `daemon` command at all — `mino daemon` reports `unknown
command`. The whole feature lives in `github.com/codyconfer/mino/daemon` and is
linked by exactly one file, `experimental_daemon.go`, a blank import behind the
tag. That package's `init()` registers the `daemon` command tree with `cmd`, the
daemon status chip with `statusstrip`, and the `daemon.tray` setting plus the
`daemon` status-bar entry with `views`; nothing in core refers back to it.

What the tag adds: `mino daemon` and its
`install/uninstall/start/stop/restart/status/attach` subcommands, the system
tray, the daemon status chip in `deck`, the `daemon.tray` setting, and the
`kardianos/service` + systray dependencies. Verify the default build carries
none of it:

```sh
go list -deps .              | grep -E 'kardianos|systray'   # empty
go list -deps -tags daemon . | grep -E 'kardianos|systray'   # both present
```

What works either way — the tag changes nothing here: every cli directive,
`deck`, and `mino serve` (the foreground realtime watcher) with its event
socket, desktop notifications, scheduled delivery, the attach notification
inbox, and `deck`'s silent background serve provider. Service-only plugin
contributions also stay available: `plugin.ServiceAttached` keys off the serve
socket rather than the installed service, so NTR reminders show up whenever
something is watching.

Both configurations are checked. `go build ./...` and `go test ./...` compile and
test the daemon package itself in the default build; `make build-experimental`
(part of `make check`) additionally builds and vets the root binary with
`-tags daemon`.

Signal integrations live in `internal/signals/<name>/`, each with offline table
tests driven by recorded fixtures, so the suite needs no network. When no live
provider is already listening, `deck` starts `serve` as a silent session-owned
background process (stdio discarded; logs still go to the log dir; dies with
deck).

Mino's reusable foundations are the public modules
[`github.com/codyconfer/sisyphus`](https://github.com/codyconfer/sisyphus) and
[`github.com/codyconfer/viewkit`](https://github.com/codyconfer/viewkit). CI and
published consumers build against the versions pinned in `go.mod`, using the
standard Go module proxy and checksum database with no private credentials or
`replace` directives.

## Local multi-repo development (`go.work`)

For simultaneous edits across mino / sisyphus / viewkit (and optionally the
overlay siblings), use an **uncommitted** `go.work` in this repo (gitignored; do
not commit — committed `replace` is rejected). A common local pattern is
`go.work.local` (also gitignored) activated with `GOWORK=go.work.local`:

```sh
# from the mino checkout, with sisyphus and viewkit as siblings:
go work init . ./external/plugins ../sisyphus ../viewkit
# or: go work use . ./external/plugins ../sisyphus ../viewkit
```

Published consumers and CI ignore `go.work` and resolve the pinned module
versions in `go.mod`. Deck lives in the single `viewkit` module (import path
unchanged: `github.com/codyconfer/viewkit/deck`); `go.mod` excludes retired
nested `viewkit/deck` module versions so the parent wins.
