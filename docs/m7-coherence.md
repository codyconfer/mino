# M7 coherence checkpoint

**Branch:** `m7-coherence` (local only)  
**Base:** M4 review-fixes + M5 viewkit/deck + M6 app.Run scaffold

## Done

| Item | Notes |
|---|---|
| Integration tip | M5 merged onto M4 ack/schedule/kubectl; M6 already ancestor |
| Config split | `types.go` / `access.go` / `settings.go` / load in `config.go` |
| Report styles | `render.ReportStyles` — verify + onboard |
| Lipgloss off errs/log | `errs.Render` + `log` tags via `theme.Cur()` (ColorNever plain) |
| Slim `cmd/ntr` | CRUD/catch-up in `internal/plugin/ntr/cli.go` |
| Deck Scroll | `viewkit/deck.Scroll`; munin `Content` is a thin wrapper |
| Deck ItemList / HomeShell | Stock widgets; munin `Results` / `Home` are thin Fetch+Bind adapters |
| Defaults → install | `Options.Defaults` → `SetDefaultsFS` → Install/Nuke merge (overlay wins) |
| CI sketch | ubuntu + macos + windows (windows `continue-on-error` until proven green) |
| Docs | this charter + viewkit deck INTERFACE API sync |

## Deferred (documented)

| Item | Why |
|---|---|
| login flow / audit / serve → stock deck | Domain forms, SQL viewport, event inbox — keep custom; provider list already uses `NewMenu` |
| viewkit package consolidation | `browser`/`timefmt` merge needs re-export plan; see `viewkit/deck/INTERFACE.md` |
| Windows CGO hard gate | Keep `continue-on-error` until a green `windows-latest` run is observed; no local Windows runner here |

## Invariants (keep)

- `sisyphus` ⊥ `viewkit`
- No committed `replace`; `go.work` uncommitted
- Tea only in `viewkit/deck` module for shared hosts; munin adapters stay thin
- CapAction / Scheduled Ack / in-process kubectl Switch stay as M4 review contracts
- Deck must not import munin `signals` — Bind/adapters map domain → `list.Item`

## Remaining / exit

- Soak tests (deck / ntr / viewkit/deck) green on this tip
- Local **v1** tags after soak — do not push
- Harden windows CI when a green matrix cell is proven

## Local tags

Prefer annotated `m7-coherence` on tips before any `v1.0.0`. Never `git push --tags`.
