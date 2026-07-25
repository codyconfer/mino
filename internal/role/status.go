package role

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/codyconfer/sisyphus/lifecycle"

	"github.com/codyconfer/munin/internal/config"
)

// StatusTextMax is the maximum display length for a status block's stdout.
const StatusTextMax = 20

// CaptureFunc executes a shell script and returns captured stdout.
type CaptureFunc func(kind, script string) (string, error)

// Capture is the process-level stdout-capturing runner; tests may replace it.
var Capture CaptureFunc = defaultCapture

// Chip is one active role status-bar contribution.
type Chip struct {
	// Glyph is the configured glyph name (tool logo id or registry id).
	Glyph string
	// Text is the truncated command output for display.
	Text string
	// Index is the block index in RoleDef.Status (stable hide-preference key).
	Index int
}

var (
	statusMu    sync.RWMutex
	statusChips []Chip
)

// StatusChips returns a copy of the active role's status chips.
func StatusChips() []Chip {
	statusMu.RLock()
	defer statusMu.RUnlock()
	if len(statusChips) == 0 {
		return nil
	}
	out := make([]Chip, len(statusChips))
	copy(out, statusChips)
	return out
}

// SetStatusChips replaces the active role status chips.
func SetStatusChips(chips []Chip) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if len(chips) == 0 {
		statusChips = nil
		return
	}
	statusChips = append([]Chip(nil), chips...)
}

// ClearStatusChips drops all role status chips (role exit / clear).
func ClearStatusChips() {
	SetStatusChips(nil)
}

// TruncateStatus trims stdout for status-bar display: leading/trailing
// whitespace removed, first line only, then at most StatusTextMax runes.
func TruncateStatus(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if utf8.RuneCountInString(s) <= StatusTextMax {
		return s
	}
	return string([]rune(s)[:StatusTextMax])
}

// CollectStatus runs each status block's platform command (same Select rules
// as hooks), captures stdout, and truncates for display. Failures are returned
// as warnings; they do not abort collection of other blocks.
func CollectStatus(rd config.RoleDef) (chips []Chip, warnings []error) {
	for i, block := range rd.Status {
		kind, script, ok := Select(block.Hooks())
		if !ok {
			continue
		}
		out, err := Capture(kind, script)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("status[%d] (%s): %w", i, displayGlyph(block.Glyph), err))
			continue
		}
		text := TruncateStatus(out)
		if text == "" && strings.TrimSpace(block.Glyph) == "" {
			continue
		}
		chips = append(chips, Chip{
			Glyph: displayGlyph(block.Glyph),
			Text:  text,
			Index: i,
		})
	}
	return chips, warnings
}

func displayGlyph(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "status"
	}
	return name
}

func defaultCapture(kind, script string) (string, error) {
	switch kind {
	case "bash":
		return captureBash(script)
	case "powershell":
		return capturePowerShell(script)
	default:
		return "", fmt.Errorf("unknown shell kind %q", kind)
	}
}

func captureBash(script string) (string, error) {
	bin, err := lifecycle.LookBash()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "-c", script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bash status: %w", err)
	}
	return stdout.String(), nil
}

func capturePowerShell(script string) (string, error) {
	bin, err := lifecycle.LookPowerShell()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("powershell status: %w", err)
	}
	return stdout.String(), nil
}
