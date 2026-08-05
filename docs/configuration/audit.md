# Audit trail

Every flight, query, and write is recorded — with timestamps, per-signal timing,
item counts, errors, and the items themselves — into `.data/audit.duckdb`. This is an
audit trail for tracking your workflow and metrics over time, **not a cache**:
results are never read back to answer a live query.

```sh
mino history                 # list recent runs (flights, queries, writes)
mino history show 12         # recall a past run's stored results
```

The file is queryable directly for ad-hoc metrics:

```sql
SELECT name, kind, count(*) AS runs, coalesce(sum(count), 0) AS items
FROM runs GROUP BY name, kind ORDER BY runs DESC;
```

Disable the trail with `audit.enabled: false` (or `MINO_AUDIT_ENABLED=false`).
Config versioning is separate — tracked in `.data/config.duckdb` and surfaced via
`mino config history`.
