# M6 — Distribution kit (scaffold)

**Branch:** `m6-distribution-kit` (local)  
**ADR:** ADR-8 (public `app` pkg · `go:embed` defaults · thin overlay repos)  
**Status:** Early scaffold — public API + template overlay; install wiring and externals graduation deferred.

## Why

Everything product-facing lived under `munin/internal/*`, so team distributions could not import a stable entrypoint (deficiency D14 / goal G7). M6 opens a **public** package and a thin overlay pattern without rewriting the plugin/deck SDK mid-flight.

## Public import path

```text
github.com/codyconfer/munin/app          → Run(opts) + hooks + ListDefaults
github.com/codyconfer/munin/app/defaults → reference go:embed seed FS (optional)
github.com/codyconfer/munin/cmd          → CLI root (wired via Options.CLI)
```

Do **not** import `github.com/codyconfer/munin/internal/...` from overlays.

## Overlay binary shape

```go
package main

import (
	"context"
	"embed"
	"io/fs"
	"os"

	muninapp "github.com/codyconfer/munin/app"
	"github.com/codyconfer/munin/cmd"
)

//go:embed all:defaults
var defaultsRoot embed.FS

func main() {
	defaultsFS, err := fs.Sub(defaultsRoot, "defaults")
	if err != nil {
		panic(err)
	}
	err = muninapp.Run(muninapp.Options{
		Defaults:    defaultsFS, // merged into Install/Nuke seeds (overlay wins)
		EmailDomain: "example.com",
		EnforceAuth: true,
		RegisterPlugins: func() {
			// plugin.Register(...) or call your plugins' Register()
		},
		CLI: func(ctx context.Context, args []string) error {
			root := cmd.Root()
			root.SetArgs(args)
			defer cmd.Shutdown()
			return root.ExecuteContext(ctx)
		},
	})
	if err != nil {
		os.Exit(1)
	}
}
```

`app` does **not** import `cmd` (avoids coupling while M5 moves term/deck). Callers wire `Options.CLI`. Stock munin `main.go` does the same after UI bootstrap.

## Build policy (domain lock / enforce auth)

| Concern | `app.Options` | ldflags (Makefile today) |
|---|---|---|
| Email domain | `EmailDomain: "example.com"` | `-X '…/internal/app/onboard.RequiredEmailDomain=example.com'` |
| Hard-block auth | `EnforceAuth: true` | `-X '…/internal/app/onboard.EnforceAuthorized=true'` |

## go:embed defaults layout

```text
defaults/
  config.yaml
  queries/*.yaml
  filters/*.yaml
  flights/*.yaml
  roles/*.yaml
```

Reference: [`app/defaults`](../app/defaults). Validate with `app.ListDefaults(fs)`.

**Deferred:** feeding `Options.Defaults` into `lifecycle.InstallSpec` / `munin install`.

## Local template repo

Sibling (private/local only — not published):

```text
../munin-overlay-template/
```

Uses `replace` → local munin checkout. No GitHub remote.

## How a team distro depends on munin

1. Private module (e.g. `github.com/example/team-munin`).
2. `require github.com/codyconfer/munin vX.Y.Z` (or `replace` → local path).
3. `main` calls `munin/app.Run` with team `RegisterPlugins`, embed FS, and `CLI`.
4. Compile-time plugin registration only (ADR-7).
5. Ship the binary; users still use `~/.munin` for runtime config.

Until munin tags a release that includes `app/`, keep the `replace` to a local clone on branch `m6-distribution-kit` (or later main).

## Deferred

| Item | Why |
|---|---|
| Externals graduate from `plugins-external` | Needs ownership + publish |
| Demo flight → live GitHub URLs | Product decision (G8) |
| `Options.Defaults` → `munin install` | Touches provision; keep M4/M5 cold |
| Publish overlay template | Explicitly out of scope |
| Push / releases | Local-only |

## Invariants

- Do not break M3/M4 SDK under `internal/plugin`.
- Prefer additive public surface over rewriting deck/NTR mid-flight.
- No push / no public repo creation from this lane.
