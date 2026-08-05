# Formatters

A **formatter** is a `type: formatter` document holding one Go
[`text/template`](https://pkg.go.dev/text/template) that turns a run's results
into a text report — a standup post, a triage digest, a canned PR or Slack reply.
The rendered text **replaces** the git-tree panels (or the JSON) on stdout, so it
pipes cleanly. New formatters land in `~/.mino/formatters/`, one per file, but
like every directive one may live anywhere under `~/.mino/`:

```yaml
# ~/.mino/formatters/standup.yaml
name: standup
type: formatter
title: Daily Standup         # optional display label
template: |
  ## Standup {{ now | date "2006-01-02" }}
  {{ range .Sections }}
  ### {{ .Title }} ({{ len .Items }})
  {{ range .Items }}- [{{ .Title }}]({{ .URL }}) {{ .Meta.author }}
  {{ end }}{{ end }}
```

Attach one with a `formatter: <name>` field on a query or flight, or choose one
per run with `--formatter`:

```sh
mino fly triage --formatter triage-summary        # ad-hoc
mino fly morning --formatter standup --copy       # render to the clipboard
mino query my-open-prs --formatter pr-nudge --out nudge.md
mino github query -F pr-nudge                     # ad-hoc single-signal query
mino formatter                                    # list what the role can see
mino formatter show standup                       # print the definition
mino formatter render standup morning             # run flight `morning`, render it
mino fly morning -o json | mino formatter render standup --stdin
```

`--formatter` beats the directive's own `formatter:` field. Without `--copy` or
`--out <path>` the report goes to stdout; `--copy` puts it on the clipboard and
`--out` writes it to a file. `render --stdin` reads a `-o json` section array
instead of running anything, so a captured result can be re-rendered.

Within a flight, per-query `formatter:` fields are **ignored** — the flight's
formatter sees the whole result set, so exactly one template renders a run.
`mino serve` and the streaming path ignore formatters entirely: a stream never
has "all the results".

In the deck, formatters and the reports they produce are one screen:
**Directives → Reports**. `render on` is the first field, so the template and the
flight it renders over sit together — `ctrl+r` runs that flight and shows the
rendered report in the results panel, from the draft in the form rather than the
file on disk, so an edit (or a formatter you have not saved yet) renders as typed.
`ctrl+g` copies the rendered text, `ctrl+w` writes it to
`<home>/reports/<name>-<timestamp>.md`, and `ctrl+s`/`ctrl+x` save and delete the
document itself.

## The template data model

The template is executed against one report value:

| Field | Is |
|---|---|
| `.Formatter` | the formatter's name |
| `.Name` | the flight or query the run was rooted at |
| `.Kind` | `"flight"` or `"query"` |
| `.Role` | the active role, empty when none |
| `.Now` | the run timestamp |
| `.Count` | total item count |
| `.Errors` | `[]string`, one entry per section that failed |
| `.Sections` | flat list of sections; each has `.Query .Signal .Title .Items .Meta .Err .Count` |
| `.Queries` | the same data grouped per source query; each has `.Query .Title .Sections .Items .Count` |
| `.Items` | every item, fully flattened; each has `.Kind .Title .Subtitle .Body .URL .Timestamp .Meta .Query .Signal` |

So `range .Queries` gives one block per saved query, `range .Sections` one per
signal section, and `.Items` the whole run as one list to bucket or sort.

A **missing map key renders empty rather than erroring** (`missingkey=zero`),
because `.Meta` is sparse and per-signal — a GitHub item has no `channel`, a
calendar event has no `author`. This is a deliberate difference from query-param
templates, which *do* fail on a missing key. A typo'd struct field
(`.Titel`) still fails the render.

## Template functions

Every function takes the piped value **last**, so `{{ now | date "2006-01-02" }}`
reads in the order it runs:

| Function | Signature |
|---|---|
| `now` | `() time.Time` |
| `date` | `(layout string, t time.Time) string` |
| `rel` | `(t time.Time) string` — `3h ago` |
| `meta` | `(key string, m map[string]string) string` |
| `default` | `(fallback, v string) string` |
| `trim` / `upper` / `lower` / `title` | `(string) string` |
| `join` | `(sep string, xs []string) string` |
| `indent` | `(n int, s string) string` |
| `truncate` | `(n int, s string) string` — rune-safe |
| `count` | `(any) int` |
| `limit` | `(n int, items) []Item` |
| `byMeta` | `(key string, items) []Bucket` — `Bucket{Key, Items}`, sorted by `Key` |
| `withMeta` | `(key, val string, items) []Item` |
| `sortByTime` | `(items) []Item` — newest first |

```
{{ range .Items | sortByTime | limit 5 }}- {{ .Title | truncate 70 }} · {{ rel .Timestamp }}
{{ end }}
{{ range byMeta "repo" .Items }}{{ .Key | default "(none)" }} — {{ len .Items }}
{{ end }}
```

Templates are parsed when directives load, so a template that will not compile is
reported by name up front rather than failing mid-render. Roles scope which
formatters are visible with `formatters:`. Copy-paste starters live in
[`examples/formatters/`](../../examples/formatters/).
