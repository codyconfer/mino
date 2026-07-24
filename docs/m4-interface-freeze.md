# M4 interface-freeze checkpoint

**Branch lineage:** `m3-sdk-gate` → `m3-sdk-gate-fixes` (local only)  
**Status:** SDK exercised by NTR (Query/Action/Scheduled) + external stubs + gcx C-0
(auth/context/actions offline) + Lane D one-per-file examples.

## Frozen for fan-out

| Surface | Package | Notes |
|---|---|---|
| Capabilities | `plugin.CapQuery/Action/Stream/Scheduled` | ADR-6/10 |
| Registry + enable | `plugin.Register` / `SetEnabled` | compile-time only |
| CapAction host | `plugin.RegisterAction` + `build.Action` + `munin action` | verify fails if CapAction w/o bindings |
| Verify | `verify.Plugins` + query builder sync | ADR-7/13 |
| Context | `ContextProvider` + `role.contexts` | ADR-9; applied on `App.Load` |
| Glyph variants | `viewkit/glyph.Register` + `StatusChip` tone | ADR-12 |
| Plugin stores | `sisyphus/store.Open` + `BackupPaths` / `plugin.DataPaths` | ADR-11 |
| Scheduler → notify | `ReminderJob` → `scheduledEvents` → serve FanIn → `notifySink` | ADR-10 |
| Scaffold | `internal/plugin/scaffold` | ADR-14 (CapQuery only) |
| Directives | `examples/{queries,filters,flights,roles}/` | one file per directive |

## Notify wire (NTR ↔ daemon)

`munin serve ntr` (or any flight that includes an `ntr` query) registers
`ntr.ReminderJob` into the same FanIn as active streams. Fired reminders become
`signals.Event` with item kind `alert`, which:

- sets tray state to warn (`stateForEvent`)
- emits desktop notifications when `--desktop` / `daemon.desktop`
- prints terminal toasts (+ bell) when no tray is attached

CLI catch-up: `munin ntr catch-up` stamps the same `serve.duckdb` watermark KV.

## Lane D examples

See [`examples/README.md`](../examples/README.md). Roles demonstrate `contexts:`;
flight `plugins` smokes Lane C stubs; flight `ntr` is the Scheduled notify path.

## Still deferred (do not churn stubs against these)

- View/panel/component registries → **M5** with `viewkit/deck`
- NTR TUI CRUD forms → **M5**
- gcx live IRM HTTP → Lane C-1 after C-0 (vertical: irm-incidents; `internal/plugin/external/gcx/SPIKE.md`)
- Distribution overlays → **M6**

Stubs remain template instantiations; deepen only after M5 view registration lands.
