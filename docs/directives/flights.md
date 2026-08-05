# Flights

A **flight** ("fly" plays on the bird) is a named, ordered list of saved query
names run concurrently — your whole shift-start sweep in one command. `mino fly
<name>` runs one; bare `mino fly` runs the active role's first flight (or
`default`), or lists what's available.

The seeded `default` flight shows open pull requests and the latest CI run for
`codyconfer/sisyphus`, `codyconfer/viewkit`, and `codyconfer/mino`. Select a CI
run to inspect its job and step statuses. The seeded `default` role uses this
flight as its home screen.

## Create a flight config

A flight is a `type: flight` document — a named, ordered list of saved query
names. New flights land in `~/.mino/flights/`, one per file, but any file under
`~/.mino/` will do:

```yaml
# ~/.mino/flights/triage.yaml
name: triage                 # run by `mino fly triage`
type: flight
queries: [incidents, my-open-prs]
```

Each entry in `queries:` refers to a query document by `name`, not by filename or
directory. A query that fails to build (missing auth, missing channel, …) shows up
as an inline error section rather than aborting the flight.
