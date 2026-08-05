# Getting started

Build (or install) the binary:

```sh
go build -o mino .
# or: go install github.com/codyconfer/mino@latest
```

Bootstrap a config directory (with a sample query, filter, and flight), then run:

```sh
mino install                 # create ~/.mino with defaults
mino onboard                 # one-time: GitHub auth + a GitHub-verified GPG key
mino fly                     # run the default flight
mino github query            # ad-hoc: your open PRs + review requests
mino fly morning -o json | jq .
```

On first use Mino guides you through [onboarding](configuration/onboarding.md) — GitHub auth plus a
GitHub-verified signing key. How it gates depends on the mode: `mino deck` runs the
guided flow; a bare `mino <directive>` prompts to authenticate when you're
unauthenticated, and otherwise warns about any remaining gaps and continues. A
binary compiled with `ALL_OR_NOTHING_AUTH=1` instead blocks ordinary cli
directives while the authenticated account remains unauthorized. Domain locking
can add an authorization requirement, but does not enable blocking by itself.
`login`, `verify`, and `--help` are always available.

Mino reuses tools you already have for authentication — the `gh` CLI, `gcloud`
ADC, `$SLACK_TOKEN` — and falls back to `mino login <service>` when they are
absent. See [Authentication](configuration/authentication.md) for the full resolution order and
required scopes.

## How it works

```
signals (fetch) ──▶ filters (regex include/exclude) ──▶ renderer (terminal | json | formatter)
```

Each signal normalizes its data into a common item shape, filters narrow the
results, and a renderer prints them. A **query** binds a signal + params +
filters under a name; a **flight** runs a named set of queries concurrently, and
one failing query degrades to an inline error section instead of blanking the
rest. Every run is recorded to a local audit trail (see
[Audit trail](configuration/audit.md)).
