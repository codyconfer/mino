# Query DuckDB from the deck

**DuckDB** is a top-level deck entry for read-only SQL against mino's `audit`,
`config`, and `tokens` databases. Its first row creates an ad-hoc query; saved
queries follow beneath it. Both paths open the same responsive editor used by
roles and other directives, so run, validate, YAML preview, save, rename, delete,
and form/results focus use the standard keybindings above.

Saved SQL is a `type: duckdb` directive under `duckdb/` by default:

```yaml
name: recent-runs
type: duckdb
database: audit
sql: |
  SELECT name, kind, count(*) AS runs
  FROM runs
  GROUP BY name, kind
  ORDER BY runs DESC
```

Only one `select`, `with`, `pragma`, `describe`, or `show` statement is accepted.
Results run off the UI loop and appear in the editor's framed, scrollable results
panel, leaving the SQL visible for quick changes and reruns.
