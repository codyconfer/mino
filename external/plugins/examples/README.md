# Overlay example directives

Directives for the signals in this module. Copy into `~/.munin` (queries into
`queries/`, flights into `flights/`) when running the overlay binary.

| File | Signal |
| --- | --- |
| `queries/today.yaml` | calendar |
| `queries/unread-mail.yaml` | gmail |
| `queries/recent-docs.yaml` | docs |
| `queries/slack-standup.yaml` | slack |
| `queries/notify-smoke.yaml` | demo (synthetic notify toasts) |
| `flights/morning.yaml` | calendar + gmail + github |
| `flights/notify-smoke.yaml` | demo |
