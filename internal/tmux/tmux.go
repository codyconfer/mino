package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	bin         = "tmux"
	SessionName = "munin"
)

type PaneID string

func Available() bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func Inside() bool { return os.Getenv("TMUX") != "" }

func SelfPane() PaneID { return PaneID(os.Getenv("TMUX_PANE")) }

func Launch(self string, args []string) error {
	argv := append([]string{"new-session", "-A", "-s", SessionName, "--", self}, args...)
	cmd := exec.Command(bin, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return errs.Wrapf(errs.KindInternal, err, "tmux new-session %s", SessionName)
	}
	return nil
}

type SplitOpts struct {
	Target     PaneID
	Horizontal bool
	Size       int
	Title      string
	Env        []string
	Argv       []string
}

func SplitArgs(o SplitOpts, envFlag bool) []string {
	argv := []string{"split-window", "-d", "-P", "-F", "#{pane_id}"}
	if o.Horizontal {
		argv = append(argv, "-h")
	} else {
		argv = append(argv, "-v")
	}
	if o.Size > 0 {
		argv = append(argv, "-l", strconv.Itoa(o.Size))
	}
	if o.Target != "" {
		argv = append(argv, "-t", string(o.Target))
	}
	cmdv := o.Argv
	if len(o.Env) > 0 {
		if envFlag {
			for _, e := range o.Env {
				argv = append(argv, "-e", e)
			}
		} else {
			cmdv = append(append([]string{"env"}, o.Env...), cmdv...)
		}
	}
	return append(append(argv, "--"), cmdv...)
}

func Split(o SplitOpts) (PaneID, error) {
	if len(o.Argv) == 0 {
		return "", errs.New(errs.KindInternal, "tmux: split needs a command")
	}
	out, err := output(SplitArgs(o, true))
	if err != nil && len(o.Env) > 0 {
		out, err = output(SplitArgs(o, false))
	}
	if err != nil {
		return "", errs.Wrap(errs.KindInternal, err, "tmux split-window")
	}
	id := PaneID(strings.TrimSpace(out))
	if id == "" {
		return "", errs.New(errs.KindInternal, "tmux split-window returned no pane id")
	}
	if o.Title != "" {
		_ = SetTitle(id, o.Title)
	}
	return id, nil
}

func Exists(id PaneID) bool {
	if id == "" {
		return false
	}
	return run("display-message", "-pt", string(id), "#{pane_id}") == nil
}

func PaneSize(id PaneID) (width, height int, ok bool) {
	args := []string{"display-message", "-p", "#{pane_width} #{pane_height}"}
	if id != "" {
		args = []string{"display-message", "-pt", string(id), "#{pane_width} #{pane_height}"}
	}
	out, err := output(args)
	if err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d %d", &width, &height); err != nil {
		return 0, 0, false
	}
	return width, height, width > 0 && height > 0
}

func Kill(id PaneID) error {
	if id == "" {
		return nil
	}
	if err := run("kill-pane", "-t", string(id)); err != nil {
		return errs.Wrapf(errs.KindInternal, err, "tmux kill-pane %s", id)
	}
	return nil
}

func SetTitle(id PaneID, title string) error {
	return run("select-pane", "-t", string(id), "-T", title)
}

func Select(id PaneID) error {
	return run("select-pane", "-t", string(id))
}

func run(args ...string) error { return exec.Command(bin, args...).Run() }

func output(args []string) (string, error) {
	b, err := exec.Command(bin, args...).Output()
	return string(b), err
}
