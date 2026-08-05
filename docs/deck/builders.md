# Build and manage directives without writing YAML

**Directives** is the deck's first menu entry, and it holds one screen per kind of
document, in order: **Flights**, **Queries**, **Roles**, **Reports**, and
**History** (which appears once something has been recorded).

Every one of the first four is the whole surface for those saved documents,
wherever they live: **New** first, then every saved document with a one-line
summary. There are no sub-screens — picking any entry opens one builder view, on a
blank document or on that one, and everything happens there by keybinding:

| key | does |
|---|---|
| `ctrl+r` | run the document as it currently stands |
| `ctrl+t` | validate it and show the findings inline |
| `ctrl+y` | toggle a YAML panel showing exactly what would be saved |
| `ctrl+s` | save (needs a name) |
| `ctrl+g` | copy the last run's text (among the directives, reports only — the **Notes** record editors bind it too) |
| `ctrl+w` | write the last run's text under `<home>/reports` (reports only — the record editors do not bind it) |
| `ctrl+x` | delete, with a confirmation dialog (saved documents only) |
| `tab` | move focus between the form and the results |
| `esc` | back |

On the **Buckets** index and inside a bucket the row keys differ again:

| key | does |
|---|---|
| `f` | on flight results, a history run, or an item detail: file that item or run into a bucket |
| `e` | rename the selected bucket (Buckets index only) |
| `ctrl+x` | on the Buckets index, delete the bucket and keep its records; **inside** a bucket, unfile the selected record and keep it. On a record list the same key deletes the record itself — the one place `ctrl+x` is not destructive is inside a bucket |

Validation runs against what's in the form, not the file on disk, so it catches
problems in edits you haven't saved yet — for a directive, an unknown signal, a
disabled plugin, a filter reference that doesn't resolve, a regex that won't
compile.

Results land in a scrollable panel under the form, not on a separate screen, so
the query that produced them stays in front of you. Focus moves to the results
when a run finishes; `tab` goes back to the form, and `↑/↓`, `pgup/pgdn`, and
`enter` (open the item's link) work on the results while they hold focus.

Both panels are sized to fit the terminal. The form scrolls around the focused
field, marking clipped edges with `⋯`, and the results take the rows their
content needs. When even that will not fit, the panel that does *not* have focus
collapses to a one-line summary; `tab` expands it again. Below roughly 20 rows
the deck's own header and footer leave too little for the builder to lay out
usefully.

Every one of these screens shares the same shell — one document type per kind
behind one editor — so across the directives the keys, the results panel, and the
save/delete behaviour are identical; only the fields differ. A flight takes
an ordered comma-separated list of query names, checked against your saved queries
before it will run or save. **Notes** reuses the same editor through a second,
plugin-local document type, so it inherits the shell but not every key or
behaviour, and its lists reload from the record store rather than being menus
built once from the files on disk. **Settings → Config** is the same shell over
`config.yaml`: `ctrl+y` previews the YAML it will merge, `ctrl+t` and `ctrl+r`
report the same checks `mino verify config` prints, `ctrl+s` writes the file
(creating `config.yaml` when there isn't one), and `ctrl+x` deletes it.

**History** is the one entry that is not a builder: past runs cannot be edited, so
selecting one opens its recorded results with `r` to refresh and `ctrl+x` to drop
the run (with the same confirmation dialog). Deleting removes the run, the queries
rolled up under it, and their recorded items.

In the query builder `type:` is the first field — `query` or `filter` — and the
rest of the form follows it: `type: filter` drops `signal`, its params, `extra
params`, and `filters` entirely, because a filter document cannot have them.
`type: query` keeps them and requires a signal. Saving always writes the `type:`
line, since a document without one does not load.

Within a query, picking a signal with `←/→` swaps the param fields to match, so
you get `query` and `project` for `github` but `channel` and `limit` for `slack` —
the param sets come from `plugin.RegisterQueryParams`, so a plugin's signal gets
the same treatment as a stock one.
Values you typed into fields that later get hidden are remembered for the
session, so flipping type to compare and back doesn't cost you your input.

Because a run never leaves the view, tuning a regex and re-running is just
`tab`, edit, `ctrl+r` — no retyping and no screen changes. Editing a saved document includes
renaming it: change `name`, save, and the file moves and the old name is dropped
from the store. A **Notes** record is the exception — it is identified by its row
id rather than a name, so there is nothing to rename.

What you save is what the form shows: switch a query to `type: filter` and the
saved document has no `signal:` or `params:`, so it passes the `type: filter`
validation rather than failing to load over a field you could no longer see.

The builder shows one inline rule and the params it knows about, but editing
preserves everything it cannot display: `aliases:`, `keywords:`, rules beyond the
first, and inline (unnamed) entries in `filters:` all survive a round trip.
Params the signal doesn't declare show up in the `extra params` field as `k=v`
pairs rather than being dropped.

The same thing from the shell:

```sh
mino query build --signal github --param query="is:open is:pr author:@me"
mino query build --signal slack --param channel=eng-standup --filter no-bots
mino query build --signal github --param query="is:open" --exclude "(?i)wip" --dry-run
mino query build --signal gmail --param query="is:unread" --save unread-now
```

`--param` and `--filter` repeat. `--include`/`--exclude`/`--field` add one inline
rule. `--dry-run` prints the query document instead of running it, which is the
quickest way to get a starting point for a file you will hand-edit. Without
`--save` nothing is written — the query runs and is forgotten.

Params are per-signal; `mino query build --help` lists the ones mino knows.
Anything else you pass through `--param` (or the builder's `extra params` field)
reaches the signal untouched, which is how plugin-defined params work.

Saving writes the YAML file **and** imports the `directives` row into DuckDB, so a
saved query is immediately runnable by name — no `mino apply` or restart. Because
the store versions every directive file as one row, that import also commits any
other staged edits sitting anywhere under `~/.mino/`.
