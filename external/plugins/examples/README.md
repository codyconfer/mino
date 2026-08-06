# Overlay example directives

Directives for the signals in this module. Copy into `~/.mino` (queries into
`queries/`, flights into `flights/`) when running the overlay binary.

| File | Signal |
| --- | --- |
| `queries/today.yaml` | calendar |
| `queries/unread-mail.yaml` | gmail |
| `queries/recent-docs.yaml` | docs |
| `queries/notify-smoke.yaml` | demo (synthetic notify toasts) |
| `flights/morning.yaml` | calendar + gmail + github |
| `flights/notify-smoke.yaml` | demo |

The Slack queries are not here: they ship embedded in the plugin, so
`mino plugins install external.slack` writes them into `~/.mino/queries`.
