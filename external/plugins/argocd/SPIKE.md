# argocd build-out notes

**Verdict:** promoted from a Lane C2 `stub` placeholder to a live read-only integration
against the ArgoCD REST API. Query + stream + detail. No write actions.

## 1. Tool / binary shape

| Candidate | Finding |
|---|---|
| `argocd` binary | Exists, but shelling out costs a process per poll and couples us to the CLI's config file format. Rejected. |
| ArgoCD REST API | The integration path. grpc-gateway JSON over HTTPS, stable since 1.x. |
| ArgoCD gRPC API | Same surface, needs a protobuf dependency the overlay module does not want. |

Unlike `kubectl`, `argocd` is **not** treated as a LookPath binary. The context tool name
`argocd` now means "the ArgoCD instance", the way `gcx`'s context means "the stack".

## 2. Auth

One token, one plane — simpler than `gcx`.

| Surface | Auth |
|---|---|
| `/api/v1/*` | `Authorization: Bearer <token>` |

Token provenance, in resolution order:

1. Sealed credential store, service key `argocd`.
2. `$ARGOCD_AUTH_TOKEN`, or whatever `plugins.argocd.token_env` names.

**This inverts `external/plugins/internal/slackauth`, which checks env first.** The
inversion is deliberate: a sealed token is the managed credential and should win over an
ambient env var that a shell profile may have set for the `argocd` CLI against a different
server. Do not "fix" it to match slack.

Mint a token with:

```sh
argocd account generate-token --account mino
```

The account needs a read-only RBAC policy:

```
p, role:mino, applications, get, */*, allow
g, mino, role:mino
```

There is no generic `mino token set` — the sealed store is only written by
`mino login <provider>`, and this plugin deliberately registers no login provider. The
supported path today is `token_env`.

## 3. Endpoints

Base `https://<server>/api/v1`.

| Call | Path | Failure mode |
|---|---|---|
| list | `GET /applications` | fatal |
| detail | `GET /applications/{name}` | fatal |
| resource tree | `GET /applications/{name}/resource-tree` | degrade to a note |
| revision metadata | `GET /applications/{name}/revisions/{rev}/metadata` | skip silently |

`appNamespace` is threaded onto every per-app call so apps-in-any-namespace resolve.

**`refresh` is never sent.** `GET /applications?refresh=normal|hard` forces a reconcile,
which is a write-ish side effect against the user's cluster. Its absence is asserted in
`client_test.go` and is the plugin's read-only guarantee.

`status.resources[]` rides on the Application object itself, so the detail resource
breakdown costs no extra call.

## 4. Deferred

- **Watch.** `GET /api/v1/stream/applications` emits newline-delimited
  `{"result":{"type":"ADDED|MODIFIED|DELETED","application":{…}}}` over a long-lived
  chunked response. Real, and the right end state, but it needs its own connection
  lifecycle, resume-on-`resourceVersion`, and unbounded-body discipline that
  `httpx.ReadBounded` does not cover. The stream polls instead; `metadata.resourceVersion`
  is already persisted to a cursor so the migration is a drop-in.
- **Write actions.** `sync`, `refresh`, `terminate-op`, `rollback`. Out of scope by
  decision — mutating a cluster from a shift assistant needs a write-target guard first.
- `/managed-resources` (full live/target manifests plus diff — heavyweight).
- `/events`, `/logs`, `/api/v1/projects`, `/api/v1/settings`.
- Multi-server. `plugins.argocd.server_url` is a single instance; the context provider
  records a selection but cannot yet repoint the client. A `plugins.argocd.servers` map
  keyed by context name is the natural follow-up.

## 5. TLS

No `insecure` flag. `internal/signals/github/backend.go` sets the house precedent of
refusing to send a bearer token over anything but https, and `server_url` is validated the
same way. The legitimate case behind `argocd login --insecure` is a private or self-signed
CA, which `plugins.argocd.ca_file` covers properly.

This is also why the tests use `httptest.NewTLSServer` and inject `srv.Client()` into
`Client.HTTP` rather than the more usual `httptest.NewServer`: `normalizeServerURL` refuses
plain http, so an http test server would have needed a test-only hole in the very guard
being tested. Serving TLS exercises the guard for real.

## 6. Checklist

- [x] Tool vs HTTP decided
- [x] Auth resolution order documented
- [x] Read-only guarantee pinned by a test
- [x] Hermetic fixture mappers ahead of the HTTP client
- [x] Query, stream, detail
- [ ] Watch endpoint
- [ ] Live verification against a real server with credentials
