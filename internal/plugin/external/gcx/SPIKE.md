# gcx C-0 spike findings (local, timeboxed)

**Date:** 2026-07-24  
**Branch:** `m4-gcx-spike` (local only — do not push)  
**Verdict:** No `gcx` CLI. Integration is **HTTP APIs** with **split auth**. Ship auth/context/glyph + offline status query now; first live vertical = **IRM incidents** (fixture-mapped offline). Defer Prom/Loki/explore.

## 1. Tool / binary shape

| Candidate | Finding |
|---|---|
| `gcx` binary | **Does not exist.** Munin signal/tool id only. |
| `grafana-cli` | Instance/plugin admin; not a Cloud query surface. |
| Cursor Grafana Cloud MCP | Hosted at `https://mcp.grafana.com/mcp`, **OAuth 2.1** user consent — different from munin sealed-token model. Useful inventory of capabilities (dashboards, Prom, Loki, OnCall, Incidents), **not** the munin runtime. |
| HTTP APIs | Real integration path for munin. |

**Assumption for munin:** treat `gcx` like `kubectl` (context tool name) + sealed credential key, not like a LookPath binary.

## 2. Auth shape (critical)

Auth is **not one token**. Surfaces diverge:

| Surface | Auth | Munin store implication |
|---|---|---|
| Grafana.com Cloud API (GCOM) — stacks/orgs/access policies | `Authorization: Bearer <Cloud Access Policy token>` | Optional later key or same store with `Scope` metadata (`stacks:read`) |
| Metrics / Loki / Traces query HTTP | Basic auth: `instanceID:CAP_token` | Separate from IRM; large surface — defer |
| Grafana **instance** HTTP API (dashboards, etc.) | Service account / API keys — **CAP does not authorize instance API** | N/A for C-0 |
| **IRM Incident** JSON-RPC | `Authorization: Bearer <glsa_… service account token>` against `https://{stack}/api/plugins/grafana-irm-app/resources/api/v1` | **Primary `gcx` token for Lane C** |
| OnCall / alert groups | OnCall API key **or** SA + `X-Grafana-URL` (OnCall OSS archived 2026-03-24; Cloud IRM continues) | Secondary / same SA if scoped |

**Sealed store key `gcx` (assumed):** stack **service account** token in `AccessToken`.  
**Context name (assumed):** stack host/slug, e.g. `myorg.grafana.net` (in-memory `ContextProvider` today).

No live credentials required for C-0; presence is probed via `token.Store.Get(ctx, "gcx")` only.

## 3. Query surface vs actions-only

### Feasible offline-testable query (confirmed)

1. **Status query** — auth present? + active stack context + vertical label (implemented in stub `Fetch`).
2. **IRM incident list mapping** — hermetic JSON → `signals.Section` (`MapIncidentsJSON`); no HTTP client yet.

### Recommended first live vertical (post C-0)

**IRM incidents** (`IncidentsService.QueryIncident*` / list-style RPC):

- Finite list/get model maps cleanly to munin sections.
- Auth matches sealed SA token + stack context.
- Suggested scopes/roles: Grafana SA with read (Editor only if write actions follow).
- CAP `incident:write` is a different plane (Cloud API); do not confuse with instance IRM RPC.

### Defer (boil-the-ocean)

- Prometheus / Loki / Tempo / Explore proxies
- Full dashboard CRUD
- Cloud MCP as munin dependency

### Actions (CapAction already advertised)

Stub names reserved; `Run` returns config error until HTTP client lands:

- `gcx.declare-incident`
- `gcx.add-activity`

## 4. Stub depth after this spike

| Piece | Status |
|---|---|
| Registry + CapQuery/CapAction | yes |
| Glyph + StatusContribution | yes |
| ContextProvider (stack slug) | yes |
| Token presence in Fetch | yes (wired via `buildGCX`) |
| Offline incident fixture mapper + test | yes |
| Live HTTP client | **no** (Lane C follow-up) |

## 5. Recommended scope for full Lane C

1. **C-1** — HTTP client for IRM base URL from context + Bearer from `gcx`; list open incidents → sections; fixture golden retained.
2. **C-2** — Optional GCOM `stacks:read` (CAP) to populate context picker; still no Prom/Loki.
3. **C-3** — Actions: declare incident / add activity (write SA role); confirm UX with deck later (M5).
4. **Out of Lane C** — metrics/logs explore; MCP bridge.

## Checklist

- [x] Identify tool/binary vs HTTP
- [x] Document auth matrix + sealed-key assumption
- [x] Pick vertical: IRM incidents
- [x] Hermetic fixture test
- [ ] Live client + scopes verified against a real stack (needs credentials; not blocking C-0)
