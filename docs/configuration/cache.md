# Result cache

Signal results are cached in `.data/cache.duckdb` and reused until they age past
`cache.ttl` (default `60s`), so re-running a flight — or running five queries that
hit the same backend — does not repeat the API calls. Caching happens before
filtering, so one fetch serves several differently-filtered queries.

```yaml
cache:
  ttl: 60s              # "0" disables; MINO_CACHE_TTL overrides
  detail_ttl: 5m        # per-item details (see `mino show`); "0" disables
  signals:
    github: 5m          # per-signal override; MINO_CACHE_SIGNALS_GITHUB
    calendar: 30s       # works for overlay signals too
```

Item details — the body, checks, reviews and comments behind `mino show` and the
deck's details view — are cached separately under `cache.detail_ttl`, because they
are fetched per item rather than per signal. The value resolves local-first:
`cache.detail_ttl` in this home's config, else `detail_cache_ttl` in the global
`settings.yaml`, else `5m`. An explicit `--cache-ttl` overrides both, so
`--cache-ttl 0` disables detail caching along with everything else.

```sh
mino fly work --refresh      # fetch live, then re-warm the cache
mino fly work --no-cache     # read nothing, write nothing
mino fly work --cache-ttl 5m # override the TTL for this run
mino cache stats             # what is cached, and how much is still fresh
mino cache clear github      # drop one signal; no argument drops everything
```

Only signals advertising the `cacheable` capability are cached, so signals reading
local state (`ntr`) always show writes immediately. An explicit `cache.signals`
entry overrides that either way — a duration turns caching on for any signal, `"0"`
turns it off.

If a fetch fails and a cached copy is less than 24h old, mino serves the cached
results and marks the section `(stale <age>)` instead of showing an error; JSON
output carries the same information in the section's `meta` object. Errors are
never cached. The cache is regenerable, so it is excluded from `mino backup`.
