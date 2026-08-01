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
	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/mino/internal/config"
)

const StatusTextMax = 20

type CaptureFunc func(kind, script string) (string, error)

var Capture CaptureFunc = defaultCapture

type Chip struct {
	Glyph string
	Text  string
	Index int
}

var (
	statusMu    sync.RWMutex
	statusChips []Chip
)

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

func SetStatusChips(chips []Chip) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if len(chips) == 0 {
		statusChips = nil
		return
	}
	statusChips = append([]Chip(nil), chips...)
}

func ClearStatusChips() {
	SetStatusChips(nil)
}

func TruncateStatus(s string) string {
	s = layout.FirstLine(s)
	if utf8.RuneCountInString(s) <= StatusTextMax {
		return s
	}
	return string([]rune(s)[:StatusTextMax])
}

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
