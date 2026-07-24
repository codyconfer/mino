package term

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

func Self() (string, error) {
	return os.Executable()
}

func Open(argv []string) error {
	if len(argv) == 0 {
		return errs.New(errs.KindUsage, "no command to open")
	}
	if runtime.GOOS == "darwin" {
		return openDarwin(argv)
	}
	if os.Getenv("TERMINAL") == "" && isWSL() {
		if err := openWSL(argv); err == nil {
			return nil
		}
	}
	return openLinux(argv)
}

func openDarwin(argv []string) error {
	cmd := appleEscape(shJoin(argv))
	iterm := false
	switch strings.ToLower(os.Getenv("TERMINAL")) {
	case "":
		iterm = exec.Command("osascript", "-e", `id of application "iTerm"`).Run() == nil
	case "iterm", "iterm2":
		iterm = true
	}
	if iterm {
		return run("osascript",
			"-e", `tell application "iTerm"`,
			"-e", `create window with default profile`,
			"-e", `tell current session of current window to write text "`+cmd+`"`,
			"-e", `activate`,
			"-e", `end tell`)
	}
	if err := run("osascript", "-e", `tell application "Terminal" to do script "`+cmd+`"`); err != nil {
		return err
	}
	return run("osascript", "-e", `tell application "Terminal" to activate`)
}

func openWSL(argv []string) error {
	wsl := []string{"wsl.exe"}
	if d := os.Getenv("WSL_DISTRO_NAME"); d != "" {
		wsl = append(wsl, "-d", d)
	}
	wsl = append(wsl, "-e")
	wsl = append(wsl, argv...)
	if p, err := exec.LookPath("wt.exe"); err == nil {
		return spawn(p, wsl...)
	}
	if p, err := exec.LookPath("cmd.exe"); err == nil {
		return spawn(p, append([]string{"/c", "start", ""}, wsl...)...)
	}
	return errs.New(errs.KindUsage, "no Windows terminal (wt.exe or cmd.exe) found on PATH")
}

func openLinux(argv []string) error {
	for _, t := range []string{
		os.Getenv("TERMINAL"),
		"x-terminal-emulator", "gnome-terminal", "konsole", "alacritty",
		"kitty", "wezterm", "foot", "ghostty", "tilix", "xterm",
	} {
		if t == "" {
			continue
		}
		path, err := exec.LookPath(t)
		if err != nil {
			continue
		}
		var args []string
		switch filepath.Base(t) {
		case "gnome-terminal", "tilix":
			args = append([]string{"--"}, argv...)
		case "wezterm":
			args = append([]string{"start", "--"}, argv...)
		case "kitty", "foot", "ghostty":
			args = argv
		default:
			args = append([]string{"-e"}, argv...)
		}
		return spawn(path, args...)
	}
	return errs.New(errs.KindUsage, "no terminal emulator found").
		WithHint("set $TERMINAL to your preferred terminal emulator")
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

func run(name string, args ...string) error { return exec.Command(name, args...).Run() }

func spawn(name string, args ...string) error { return exec.Command(name, args...).Start() }

func shJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = shQuote(a)
	}
	return strings.Join(parts, " ")
}

func shQuote(s string) string {
	if s != "" && allShellSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func allShellSafe(s string) bool {
	for _, r := range s {
		if !shellSafeRune(r) {
			return false
		}
	}
	return true
}

func shellSafeRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
		r == '_' || r == '-' || r == '.' || r == '/'
}

func appleEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
