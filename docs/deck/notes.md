# Build notes, tasks, and reminders the same way

**Notes** is a plugin contribution on the deck's main menu — a sibling of
**Directives**, not one of its screens — and it holds one screen per kind of
record: **Notes**, **Tasks**, and **Reminders** (the last only while a
`serve`/`daemon` socket is attached), plus **Buckets**, which groups records across
all three. Each record screen is the whole surface for that kind: **New** first,
then every record with a one-line summary, and picking either entry opens one
editor — on a blank record or on that one. The `alt+n` / `alt+t` / `alt+r` hotkeys
open the same builders from anywhere on the deck, and `alt+b` opens **Buckets**.

A record is not a file, so the keys do slightly different work:

| key | does |
|---|---|
| `ctrl+r` | run the `ntr` signal for the active role — the same fetch a `signal: ntr` query performs. On the **Reminders** editor it runs the reminder job instead, so you see what would fire right now; it never acknowledges anything |
| `ctrl+t` | validate: check that the due parses, and flag a reminder that would fire immediately (already past due) or never (no due, or already done); for a note it just reports the body it would save |
| `ctrl+y` | toggle a YAML panel showing the record |
| `ctrl+s` | save (needs a **title**, not a name) |
| `ctrl+g` | copy the record, with no prior run needed — a record already is text, where a report has to be rendered first |
| `ctrl+x` | delete, with the same confirmation dialog (saved records only) |
| `tab` | move focus between the form and the results |
| `esc` | back |

`ctrl+w` is deliberately unbound here: a record has no file output.

The rest of the deltas against the directive builders:

- Identity is the row **id**, not a name — the editor of a saved record is titled
  `edit note #3` — so there is no rename: changing the title changes the title and
  nothing else. The first `ctrl+s` on a builder turns that same view into the
  editor of the record it just created, so the next `ctrl+s` updates it instead of
  saving a second copy.
- Task `done` is an ordinary editable toggle. Reminder `done` is **read-only in
  the editor**, and one-way on the list: `reminders.done` is the flag the daemon
  reads to keep from notifying you twice, so un-doning is not offered.
- On a list, `enter` edits the row, `x` toggles done (tasks) or marks done
  (reminders, which then drop off the list of open ones), and `r` refreshes. The
  list also refreshes itself when you come back from an editor that saved or
  deleted. It does **not** currently notice a `mino notes add` run by another
  process — press `r` for that.
- A multiline note body cannot take a typed newline in the TUI — a pre-existing
  viewkit form limitation, not a new one. Use `mino notes add <title> <body>` or
  `mino notes update <id> <title> <body>` when the body needs more than one line.

## Buckets group records, and can be anchored

A **bucket** is a named, role-scoped container that notes, tasks, and reminders are
filed into. Membership is many-to-many: one note can sit in a hand-named bucket
*and* against the pull request that prompted it. Deleting a bucket only removes the
grouping — the records survive, and so does their membership in every other bucket.

Buckets come in three kinds, distinguished by what they hang off:

| kind | anchor | created by |
|---|---|---|
| `user` | none | you, from the **Buckets** index or `mino notes buckets add` |
| `item` | a result item's URL | filing against a row, on demand |
| `run` | `run:<audit id>` | filing against a recorded run, on demand |

An item's URL is its only identity in mino, so a row with no URL cannot be anchored
and `f` reports "nothing to anchor" rather than guessing. Anchored buckets are
created the first time you file against that item or run, and reused after.

Four surfaces reach them:

- **Buckets** on the notes menu — create, rename, delete, and drill into a bucket to
  see its records, toggle a task or reminder done, unfile a row, or create a new
  record straight into it.
- `f` on **flight results** and on an **item detail** — pick an existing bucket, the
  item's own bucket, or a new one, then pick note/task/reminder. The editor opens
  seeded from the item, and saving files it.
- `f` on a **history run** — the same, anchored to the run id.
- A dim `notes N` chip on any result row that already has records filed against it,
  plus a `notes` row in the detail gutter and a `notes` context cue.

Filing into a hand-named bucket from an item files the record into the item's anchor
bucket **as well**, so the `notes N` chip stays truthful — otherwise "file this PR's
note under `q3-migration`" would leave the PR reading zero.

The chip is computed per load with one query, not per row, and it is read from local
state rather than from the recorded run, because the audit journal keeps only
`signal/kind/title/subtitle/url` per item.

From the shell:

```sh
mino notes buckets add escalations
mino notes buckets list                  # id, members, kind, name
mino notes buckets show 7                # the bucket and everything filed into it
mino notes buckets file 7 12             # file record #12 into bucket #7
mino notes buckets unfile 7 12           # keep the record, drop the membership
mino notes buckets for 12                # which buckets is #12 in?
mino notes add "check the runbook" --bucket 7
```

`--bucket` also works on `mino notes tasks add` and `mino notes remind add`.
