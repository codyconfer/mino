# HTTP trigger API

`mino serve --http` (or `daemon.http.enabled: true`) exposes a small HTTP API under
`/api/v1` for as long as serve runs. Its endpoints are alternative triggers for
commands you would otherwise type: run a flight, run a saved query, run a plugin action,
or watch the event stream. It is **off unless you ask for it**.

The bind address defaults to `127.0.0.1`. `--http-host` (or `daemon.http.host`) can widen
it — that exists for containers, and it is the one setting here that removes a defence
rather than adding one. See [Container](container.md).

**Authentication.** Every route except `GET /healthz` and the two sign-in routes needs a
bearer token — either `daemon.http.token` or a session minted by
[GitHub sign-in](#github-sign-in). With
`daemon.http.token` unset, serve generates one on first use and writes it to
`<home>/.data/http.token` (mode `0600`); the startup line names that path, never the
token itself. Reuse it across restarts, or rotate by deleting the file and restarting.
While bound to loopback, requests must also address the listener by a loopback name — a
non-loopback `Host` header is rejected, which blunts DNS-rebinding from a browser on the
same machine. That check is skipped when you have deliberately bound off-box, where callers
legitimately address mino by container IP or DNS name and the token is the only credential.
There is no `?token=` query fallback, so browser `EventSource` cannot reach
`/api/v1/events`; use `curl -N`.

```sh
T=$(cat ~/.mino/.data/http.token)
curl -sS -H "Authorization: Bearer $T" -XPOST http://127.0.0.1:7717/api/v1/flights/default | jq
curl -sS -H "Authorization: Bearer $T" -XPOST http://127.0.0.1:7717/api/v1/queries/my-open-prs
curl -sS -H "Authorization: Bearer $T" -XPOST http://127.0.0.1:7717/api/v1/actions/ntr/note.add \
     -H 'Content-Type: application/json' -d '{"params":{"title":"from curl"}}'
# file an existing record against an item; the anchor bucket is created on demand
curl -sS -H "Authorization: Bearer $T" -XPOST http://127.0.0.1:7717/api/v1/actions/ntr/bucket.file \
     -H 'Content-Type: application/json' \
     -d '{"params":{"id":"12","anchor":"https://github.com/o/r/pull/7","anchor_kind":"item"}}'
curl -N  -H "Authorization: Bearer $T" http://127.0.0.1:7717/api/v1/events
```

| Method | Path | What it does |
|---|---|---|
| GET | `/healthz` | Liveness. **No auth**; reports only status, flight, uptime |
| GET | `/api/v1/status` | Flight, role, home, interval, socket, open sources, SSE clients, runs in flight |
| GET | `/api/v1/list?kind=flights\|queries` | Role-visible names; `&all=1` ignores the role scope |
| GET | `/api/v1/config` | The active config, with the API token redacted. `DefaultRole` is the config default; the session's active role is `role` in `/api/v1/status` |
| GET | `/api/v1/actions` · `/api/v1/actions/{signal}` | Registered `CapAction` bindings |
| POST | `/api/v1/flights/{name}` | Run a flight |
| POST | `/api/v1/queries/{name}` | Run a saved query |
| POST | `/api/v1/actions/{signal}/{name}` | Run an action; body `{"params":{…}}` |
| GET | `/api/v1/events` | The serve event stream as SSE |
| POST | `/api/v1/auth/device/{provider}` | Start a sign-in. **No token**; only exists with identity on |
| POST | `/api/v1/auth/device/{provider}/token` | Poll it; body `{"auth_id":"…"}`. **No token** |
| GET | `/api/v1/auth/session` | Who this credential is |
| DELETE | `/api/v1/auth/session` | Revoke the session you presented |

## GitHub sign-in

Set `daemon.http.identity.enabled: true`, a `client_id`, and `allowed_logins`, and a person
can trade a GitHub identity for a mino session instead of being handed `daemon.http.token`.
It uses the OAuth **device flow**, so nothing needs a browser or a redirect listener on the
machine running serve — which is the point, since serve is usually headless or in a
container.

```sh
A=http://127.0.0.1:7717/api/v1/auth/device/github
ID=$(curl -sS -XPOST $A -H 'Content-Type: application/json' -d '{}' | tee /dev/stderr | jq -r .auth_id)
# open the verification_uri it printed, enter the user_code
S=$(curl -sS -XPOST $A/token -H 'Content-Type: application/json' -d "{\"auth_id\":\"$ID\"}" | jq -r .session_token)
curl -sS -H "Authorization: Bearer $S" http://127.0.0.1:7717/api/v1/auth/session | jq
```

The poll returns **`202` with `{"status":"pending","interval":N}`** until the human acts —
that is not a failure, and `interval` is enforced server-side, so polling faster earns a
`429` rather than burning the shared rate budget every caller of that client id depends on.
A dead authorization — denied, expired, unknown, or already redeemed — is one
indistinguishable `410`, so a stale `auth_id` never confirms it was once real. The GitHub
`device_code` never leaves the daemon: callers hold an opaque single-use `auth_id` instead.

**This is authentication, not authorization.** Every allowed login gets exactly the power
`daemon.http.token` has — run any flight or action, read `/api/v1/config`, read the event
stream — because requests still run as the serve session's role. Adding twelve logins does
not improve least privilege; it twelve-times the number of people holding full API power.

Other things worth knowing:

- It is **additive and off by default**. The static token keeps working, so the tray, the
  compose file and any existing script are unaffected.
- **An empty `allowed_logins` with `enabled: true` is a startup failure**, not a permissive
  default. So is a malformed login, a duplicate that differs only in case, a missing client
  id, a `session_ttl` outside 1m–90d, and a non-`https` `github.api_url`.
- Sessions **survive a serve restart** (a device flow needs a human, so dropping them would
  be hostile) and are stored in `serve.duckdb` as a SHA-256 of the token, never the token.
  Editing `allowed_logins` or the client id invalidates every outstanding session on the next
  request. Deleting rows from the file under a running serve does *not* revoke — use
  `DELETE /api/v1/auth/session`, or restart.
- Sessions expire at `session_ttl` (default `12h`) and are **not renewed**; sign in again.
  For unattended long-lived access use `daemon.http.token`.
- mino asks GitHub for **no scopes at all**. Resolving a login needs none, and the GitHub
  token is used once to answer "who is this" and then discarded — it is never cached as an
  outbound credential. That is `mino login github`'s job, and the two are unrelated: one
  decides how mino talks *to* GitHub, this one decides who may talk *to mino*.
- The OAuth App needs **"Enable Device Flow"** ticked. `daemon.http.identity.client_id` is
  separate from `github.oauth_client_id` so that app does not have to carry `repo` scope, and
  rotating one does not break the other. Device flow is a public client: there is no secret.
- Revoking a session does not tear down an already-open `/api/v1/events` stream, which
  authenticated once when it connected. Same as the static token behaves today.
- The sign-in routes are the only ones that take no credential, and they still enforce the
  loopback `Host` check and require `Content-Type: application/json` (so a browser cannot
  reach them without a CORS preflight that has no answer). There are no cookies and no
  `Access-Control-*` headers anywhere in this API.

`/healthz` sits at the root, outside `/api/v1`, so a container healthcheck needs no token
and no version. A wrong method returns `405` with `Allow`; nothing redirects, so a trailing
slash is a hard `404`.

The run endpoints are `POST` because they write audit rows, write the result cache and hit
rate-limited upstreams — no browser or proxy should prefetch them.

**Response shape.** A successful run returns byte-for-byte what `-o json` prints, so
`curl … /api/v1/flights/default` and `mino fly default -o json` agree exactly. A partly-failed
flight is still `200` with the full array (matching the CLI, which prints partial results)
plus `X-Mino-Sections-Failed: <failed>/<total>`. When *every* source failed the body is
unchanged but the status is `502` and `X-Mino-Outcome: failed` is set — so `curl -f` works
and the per-section `error` fields are always readable.

**Errors** come back as `{"error":{"kind","message","hint"}}`, with `kind` from mino's own
error kinds and messages sanitized (signal errors relay remote text). `401` is reserved
for this API's credentials — a wrong token, an unknown session and an expired session all
return the same body, so a caller never learns which one mino holds. An upstream credential
failure is `kind: "auth"` with status `502`, so "your token is wrong" is never confused with
"mino cannot reach GitHub"; a sign-in refused by `allowed_logins` is `403` with the same
kind, because that is mino's own decision and not GitHub's. Unknown names are `404`, a name
outside the active role is `403`, a disabled signal is `409`, a dead sign-in is `410`, a
missing `Content-Type: application/json` on a sign-in route is `415`, and a stalled action
is `504`.

**Limits.** At most `daemon.http.max_concurrent` runs execute at once (default 4); beyond
that requests get `429` with `Retry-After`. This matters because one flight fans out to
several concurrent fetches, so an unattended `curl` loop would otherwise walk straight into
an upstream rate limit. `/api/v1/events` accepts at most 16 connections.

**The event stream is lossy**, exactly like the unix socket: a client that stops reading
misses events rather than stalling serve, and there is no replay, so you get events from
the moment you connect and `Last-Event-ID` is not honoured. The audit log is the durable
record. Note the two feeds spell a section error differently — run endpoints use `error`
(the `-o json` shape) and SSE frames use `err` (the socket shape); each matches its
existing CLI counterpart.

Requests run as the **serve session's role**; there is no per-request role override — a
session records *who* authenticated, never *what* they may do. A
taken address is a **hard startup failure**, not a silent downgrade — unlike the unix socket,
the port may belong to an unrelated program and there is no attach-to-the-existing-one
story. The provider `mino deck` starts for itself always passes `--http=false`, so it
never competes for the port.
