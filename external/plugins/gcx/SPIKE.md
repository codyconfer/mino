# gcx C-0 spike findings (local, timeboxed)

**Date:** 2026-07-24  
**Branch:** graduated to `external/plugins` (local only — do not push)  
**Verdict:** No `gcx` CLI. Integration is **HTTP APIs** with **split auth**. Ship auth/context/glyph + offline status query now; first live vertical = **IRM incidents** (fixture-mapped offline). Defer Prom/Loki/explore.

## 1. Tool / binary shape

| Candidate | Finding |
|---|---|
| `gcx` binary | **Does not exist.** Mino signal/tool id only. |
| `grafana-cli` | Instance/plugin admin; not a Cloud query surface. |
| Cursor Grafana Cloud MCP | Hosted at `https://mcp.grafana.com/mcp`, **OAuth 2.1** user consent — different from mino sealed-token model. Useful inventory of capabilities (dashboards, Prom, Loki, OnCall, Incidents), **not** the mino runtime. |
| HTTP APIs | Real integration path for mino. |

**Assumption for mino:** treat `gcx` like `kubectl` (context tool name) + sealed credential key, not like a LookPath binary.

## 2. Auth shape (critical)

Auth is **not one token**. Surfaces diverge:

| Surface | Auth | Mino store implication |
|---|---|---|
| Grafana.com Cloud API (GCOM) — stacks/orgs/access policies | `Authorization: Bearer <Cloud Access Policy token>` | Optional later key or same store with `Scope` metadata (`stacks:read`) |
| Metrics / Loki / Traces query HTTP | Basic auth: `instanceID:CAP_token` | Separate from IRM; large surface — defer |
| Grafana **instance** HTTP API (dashboards, etc.) | Service account / API keys — **CAP does not authorize instance API** | N/A for C-0 |
| **IRM Incident** JSON-RPC | `Authorization: Bearer <glsa_… service account token>` against `https://{stack}/api/plugins/grafana-irm-app/resources/api/v1` | **Primary `gcx` token for Lane C** |
| OnCall / alert groups | OnCall API key **or** SA + `X-Grafana-URL` (OnCall OSS archived 2026-03-24; Cloud IRM continues) | Secondary / same SA if scoped |

**Sealed store key `gcx` (assumed):** stack **service account** token in `AccessToken`.  
**Context name (assumed):** stack host/slug, e.g. `myorg.grafana.net` (in-memory `ContextProvider` today).

No live credentials required for C-0; presence is probed via `plugin.TokenSource.GetToken(ctx, "gcx")` only.

## 3. Query surface vs actions-only

### Feasible offline-testable query (confirmed)

1. **Status query** — auth present? + active stack context + vertical label (implemented in stub `Fetch`).
2. **IRM incident list mapping** — hermetic JSON → `plugin.Section` (`MapIncidentsJSON`); no HTTP client yet.

### Recommended first live vertical (post C-0)

**IRM incidents** (`IncidentsService.QueryIncident*` / list-style RPC):

- Finite list/get model maps cleanly to mino sections.
- Auth matches sealed SA token + stack context.
- Suggested scopes/roles: Grafana SA with read (Editor only if write actions follow).
- CAP `incident:write` is a different plane (Cloud API); do not confuse with instance IRM RPC.

### Defer (boil-the-ocean)

- Prometheus / Loki / Tempo / Explore proxies
- Full dashboard CRUD
- Cloud MCP as mino dependency

### Actions (CapAction already advertised)

Stub names reserved; `Run` returns an error until HTTP client lands:

- `gcx.declare-incident`
- `gcx.add-activity`

## 4. Depth after C-1 + C-3

| Piece | Status |
|---|---|
| Registry + CapQuery/CapAction | yes |
| Glyph + StatusContribution | yes |
| ContextProvider (stack slug) | yes |
| Token presence in Fetch (`view=status`) | yes (via public `plugin.TokenSource`) |
| Offline incident fixture mapper + test | yes |
| Live HTTP client (`view=incidents`) | yes — **wire contract unverified**, see §6 |
| `mino login gcx` (sealed SA token) | yes |
| Write actions (declare / add activity) | yes, behind `plugins.gcx.allow_write` |

## 5. Scope for full Lane C

1. **C-1** — HTTP client for IRM base URL from context + Bearer from `gcx`; list open incidents → sections; fixture golden retained. **Done.**
2. **C-2** — Optional GCOM `stacks:read` (CAP) to populate context picker; still no Prom/Loki. **Out of scope** for the C-1 + C-3 change; the stack is named by hand (param, role `contexts:`, or `plugins.gcx.stack`).
3. **C-3** — Actions: declare incident / add activity (write SA role); confirm UX with deck later (M5). **Done.**
4. **Out of Lane C** — metrics/logs explore; MCP bridge.

## 6. Implementation notes

The gcx package carries no code comments. Everything a comment would have said lives here.

### 6.1 The IRM wire contract is UNVERIFIED

No request in `irm.go` has ever been sent to a real Grafana Cloud stack. The shapes
below were derived from §2/§3 of this spike, not from a live response.

| Unverified | Where | Fix when verified |
|---|---|---|
| Method separator — `IncidentsService.Query…` vs `IncidentsService/Query…` | `rpcSep` in `irm.go` | flip one string const |
| Method names (`QueryIncidentPreviews`, `CreateIncident`, `AddActivity`) | the `rpcMethod(...)` var block in `irm.go` | edit that block only |
| List envelope key — `incidents` vs `incidentPreviews` | `incidentsEnvelope` in `incidents.go` | **no change** — both spellings decode, guarded by `TestMapIncidentsJSONAcceptsBothEnvelopes` |
| Severity field — `severity` vs `severityLabel` | `incidentWire` in `incidents.go` | **no change** — both decode, `severity` wins |
| Whether `incidentURL` is returned | `incidentWire.normalize` | **no change** — synthesized from the stack host when absent |
| Incident deep-link path (`/a/grafana-irm-app/incidents/<id>`) | `incidentWire.normalize` | one string |
| Request body field names (`query.queryString`, `summary`, `activityKind`) | `QueryIncidents` / `CreateIncident` / `AddActivity` | those three funcs |

A 404 from the RPC surfaces a hint naming this section, so a wrong path fails loudly
rather than silently.

**To verify:** point `mino gcx query --view incidents --stack <real> --limit 5` at a real
stack, capture the response into `testdata/incident_previews.json`, adjust the block
above, and tick the checklist item.

### 6.2 `LoginProvider` declares no `Fields`

`internal/app/loginflow/cli.go` prompts for every `LoginField` whose value is empty and
then calls `loginflow.PersistCredentials`, which writes those values into
`~/.mino/config.yaml` **in cleartext** — before `Login` ever runs. A `glsa_…` service
account token must not land there. With `Fields` empty the prompt-and-persist block is
skipped entirely, and `login()` reads the token itself (env → piped stdin → hidden TTY
prompt) and seals it straight into the credential store. `TestLoginProviderHasNoFields`
guards this.

Consequence: `RunCLI` early-returns when `Authed` is true, and there is no `mino logout`,
so `mino login gcx` refuses once a token is sealed or `$GCX_TOKEN` is set. `mino gcx login
--force` re-seals. A host-side `mino logout` would remove the need for it — Slack and
Google have the same limitation.

### 6.3 Actions reach the host through `hostFn`

`plugin.ActionFunc` is `(ctx, params) error` — no `BuildContext`, so no `Host`, no
credential store, no settings. `internal/pluginhost` is off limits to this module. The
only public route is `cmd.Host(signal)`, which is what `drive` and `tasks` use for their
write paths. It dereferences a package global set in the root command's
`PersistentPreRunE`, so it nil-panics inside an overlay test binary; `actions.go` wraps it
in a `hostFn` var that tests replace. `newClientFn` is the matching seam for pointing the
IRM client at an `httptest` server.

### 6.4 `allow_write` defaults to false

Declaring a production incident by accident is the failure mode worth an explicit opt-in,
so both write actions refuse until `plugins.gcx.allow_write: true`. This mirrors how
`drive` and `tasks` demand a configured writable target. Validation of required params
happens before the gate check opens any connection.

### 6.5 `CapCacheable` is deliberately omitted

One signal now serves two views. `view=status` reports local sealed-token presence —
exactly the "signals reading local state should omit it" case in `plugin.CapCacheable`'s
contract. A cached status view would keep reporting "unset" after `mino login gcx`.
Caching the incidents view is still available with no code change via
`cache.signals.gcx: 5m`, which overrides the capability check.

### 6.6 Fixture provenance

`testdata/incidents_list.json` is the C-0 golden and is unchanged.
`testdata/incident_previews.json` and `testdata/incident_created.json` are hand-written
from this spike and share the unverified status of §6.1.

## Checklist

- [x] Identify tool/binary vs HTTP
- [x] Document auth matrix + sealed-key assumption
- [x] Pick vertical: IRM incidents
- [x] Hermetic fixture test
- [x] Live client (C-1) and write actions (C-3) implemented
- [ ] Live client + scopes verified against a real stack (needs credentials)
- [ ] Confirm the RPC separator, method names, envelope key, severity field, and incident URL shape — see §6.1
