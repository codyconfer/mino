# Operating modes

Mino runs in one of four modes over the same engine. Each has a `mino` command,
a matching `make` target (which builds the binary, then runs it — pass runtime
arguments via `ARGS="…"`), and a fixed stdin/stdout/stderr contract:

| Mode | Command | `make` | What it does | stdout | Logs |
|---|---|---|---|---|---|
| **cli** | `mino <directive>` (`fly`, `query`, `github query`, …) | `make command ARGS="fly work"` | Run a directive and print the result | [viewkit](https://github.com/codyconfer/viewkit) panels (color on a TTY, plain when piped, JSON with `-o json`) | log dir |
| **serve** | `mino serve [flight]` | `make serve ARGS="work"` | Foreground realtime watcher in the current shell (Ctrl-C exits); **no OS service / tray** | live notification stream | shell **and** log dir |
| **daemon** *(experimental, `-tags daemon`)* | `mino daemon [flight]` | `make daemon ARGS="work"` | Install the OS service if missing, then start it (idempotent); optional system tray via `daemon.tray` | — | OS logging (journald / launchd / Windows service) |
| **deck** | `mino deck [flight]` (`--tmux` for a multi-pane workspace) | `make run` | Full-screen TUI only; attaches to a running daemon, else starts a **silent** background `serve` that dies with the deck session | TUI | log dir |

`make run` is deck only — it does not leave a serve process behind. `mino deck` is
the interactive front-end (formerly `mino tui`, still accepted as a hidden alias):
a main menu, run **history**, **query** and **flight** builders that build, run,
validate, save, and delete in one view each, a **directives** browser for roles,
**Notes** (notes/tasks/reminders, on those same builders), a **Plugins**
enable/disable screen, accounts, an ad-hoc read-only **audit query** screen, and
**settings**. `mino deck <flight>` jumps straight to a flight; `mino settings`
opens just the settings screens. When the background daemon is installed, its
status strip shows whether it is running.

## tmux workspace (`mino deck --tmux`)

`mino deck --tmux` runs the deck inside a tmux session (named `mino`) so it can
split off auxiliary panes on demand. Outside tmux it creates-or-attaches the
session; inside tmux it uses the current pane. Requires `tmux` on `PATH`.

The deck pane is the **owner**: it runs the background `serve` process, exactly as
it does without `--tmux`. Auxiliary panes are **thin** — they open no database at
all, which keeps them cheap to start rather than being a correctness requirement.
An inbox pane reads the owner's `serve.sock`; a popped-out view reads a JSON
snapshot the owner writes under `<home>/.data/panes/` and re-renders whenever the
owner rewrites it.

Panes are opened by hotkey. The targets ship unbound — bind them under
`keybinds:` in `config.yaml`:

```yaml
keybinds:
  alt+i: pane.inbox   # live event inbox, attached to the owner's stream
  alt+p: pane.pop     # pop the current detail or flight results into a pane
  alt+s: pane.shell   # a plain $SHELL pane
  alt+x: pane.close   # close the most recently opened pane
```

### External tools

A `tool:<name>` binding opens another terminal program from the deck. Define the
program under `tools:` and bind it like any other hotkey target:

```yaml
tools:
  k9s:
    argv: [k9s, "--context={{context.kubectl}}"]
    title: k9s            # optional; the pane title and footer hint
  lazygit:
    argv: [lazygit]
keybinds:
  alt+k: tool:k9s
  alt+g: tool:lazygit
```

Inside tmux the tool opens in a split pane, so mino stays visible beside it.
Outside tmux the deck suspends, hands over the terminal, and redraws when the
tool exits.

A tool whose binary is not on `PATH` leaves its binding **inert** — the key does
nothing and no footer hint appears — so the same `config.yaml` is portable
across machines where you have not installed everything.

`{{context.<tool>}}` expands to the context mino currently has selected for that
context tool, which is what makes `alt+k` open k9s on the same cluster the deck
is showing (`mino context switch kubectl prod`, or a role's `contexts:`
binding — see [Data signals](reference/signals.md#kubernetes)). Only the
`context.` namespace is substitutable. Write an optional flag as a **single**
`--flag={{…}}` token: if any placeholder in a token resolves to nothing, that
whole token is dropped, which is what keeps a bare `--context=` off the command
line when no context is selected.

Note that mino's read-only guarantees cover mino. A tool you launch this way is
its own program with your own credentials — k9s can delete a pod.

Splits are width-aware: mino splits side-by-side only when both panes would
still clear the deck's 80-column minimum, otherwise it stacks them. Panes are
killed when the deck exits, and they exit on their own within ~2s if the deck is
`SIGKILL`ed.

Flight and query results render as a **git-style tree** — the run is the trunk,
each signal a branch, each item a leaf — in both cli output and the deck. A row
that already has notes filed against it carries a dim `notes N` chip (see
[Buckets group records, and can be anchored](deck/notes.md#buckets-group-records-and-can-be-anchored)). The
trunk is labelled with what you ran: the flight name for `mino fly morning`, the
query name for `mino query my-prs`, the signal for `mino github query`.
