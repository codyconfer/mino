package config

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/sisyphus"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
)

type reconcileChoice int

const (
	choiceUnknown reconcileChoice = iota
	choiceApply
	choiceSession
	choiceIgnore
	choiceDiscard
	choiceEdit
)

const defaultReconcileChoice = choiceSession

type choiceSpec struct {
	choice  reconcileChoice
	key     string
	aliases []string
	label   string
	desc    string
}

var reconcileChoices = []choiceSpec{
	{choiceApply, "a", []string{"apply", "import"}, "apply changes", "write all staged files to the store"},
	{choiceSession, "s", []string{"session", "use"}, "use this session", "run with all of them, leave the store as-is"},
	{choiceIgnore, "i", []string{"ignore", "skip"}, "ignore staged", "run with the stored versions instead"},
	{choiceDiscard, "d", []string{"discard", "delete"}, "discard changes", "delete all staged files, keep stored"},
	{choiceEdit, "e", []string{"edit", "editor", "open"}, "open in editor", "open the config folder with $EDITOR, then re-ask"},
}

func reconcileChoiceFor(line string) reconcileChoice {
	s := strings.ToLower(strings.TrimSpace(line))
	if s == "" {
		return defaultReconcileChoice
	}
	for _, c := range reconcileChoices {
		if s == c.key || slices.Contains(c.aliases, s) {
			return c.choice
		}
	}
	return choiceUnknown
}

func reconcilePromptLine() string {
	th := theme.Cur()
	keys := make([]string, 0, len(reconcileChoices))
	for _, c := range reconcileChoices {
		keys = append(keys, c.key)
	}
	return th.Dim.Render("  choose ") + th.Key.Render("["+strings.Join(keys, "/")+"]") +
		th.Dim.Render(" (enter = use this session): ")
}

func discardConfirmBatchLine(home string, recs []sisyphus.Reconciliation) string {
	th := theme.Cur()
	targets := make([]string, 0, len(recs))
	for _, rec := range recs {
		if rec.Name == ConfigDirective {
			targets = append(targets, filepath.Join(home, "config.*"))
			continue
		}
		targets = append(targets, filepath.Join(home, rec.Name))
	}
	return th.Cant.Render("  "+glyph.Pad(glyph.Warn())+"delete ") + th.Val.Render(strings.Join(targets, ", ")) +
		th.Cant.Render(" from disk?") + th.Dim.Render(" [y/N]: ")
}

func renderReconcileNotice(msg string) string {
	return theme.Cur().Dim.Render("  " + glyph.Pad(glyph.Bullet()) + msg)
}

func renderReconcilePanel(w io.Writer, rec sisyphus.Reconciliation) string {
	return renderReconcileBatchPanel(w, []sisyphus.Reconciliation{rec})
}

func renderReconcileBatchPanel(w io.Writer, recs []sisyphus.Reconciliation) string {
	f := layout.FrameFor(w)
	th := theme.Cur()

	names := make([]string, len(recs))
	for i, rec := range recs {
		names[i] = rec.Name
	}
	lines := []string{
		f.Row("changed", th.Accent.Render(strings.Join(names, ", "))),
		"",
	}
	budget := f.BodyWidth() - 16
	for _, rec := range recs {
		lines = append(lines, f.Row(rec.Name, th.Val.Render(stagedSummary(rec))))
		lines = append(lines, f.Row("", th.Dim.Render(storedSummary(rec))))
		if changed := changedSummary(rec, budget); changed != "" {
			lines = append(lines, f.Row("", changed))
		}
	}
	lines = append(lines, "")
	lines = append(lines, th.Dim.Render("  one choice applies to all listed directives"))
	lines = append(lines, "")

	width := 0
	for _, c := range reconcileChoices {
		width = max(width, len(c.label))
	}
	for _, c := range reconcileChoices {
		lines = append(lines, th.Key.Render(" "+c.key+" ")+"  "+
			th.Val.Render(fmt.Sprintf("%-*s", width, c.label))+"  "+
			th.Dim.Render(c.desc))
	}

	return f.TitledBoxIcon(glyph.Pad(glyph.Warn()), "new config changes staged", lines...)
}

func stagedSummary(rec sisyphus.Reconciliation) string {
	if files, ok := collectionFiles(rec.FileContent, rec.FileFormat); ok {
		return fmt.Sprintf("%s · %s", plural(len(files), "file"), humanBytes(len(rec.FileContent)))
	}
	return fmt.Sprintf("%s · %s", rec.FileFormat, humanBytes(len(rec.FileContent)))
}

func storedSummary(rec sisyphus.Reconciliation) string {
	if !rec.HasDB {
		return "nothing stored yet"
	}
	return fmt.Sprintf("%s · applied %s", shortHash(rec.DB.Hash), rec.DB.At.Local().Format("2006-01-02 15:04"))
}

type changeKind int

const (
	changeAdd changeKind = iota
	changeMod
	changeDel
)

type changeEntry struct {
	base string
	kind changeKind
}

func changeEntries(rec sisyphus.Reconciliation) []changeEntry {
	staged, ok := collectionFiles(rec.FileContent, rec.FileFormat)
	if !ok {
		if !rec.HasDB {
			return nil
		}
		return []changeEntry{{base: rec.Name, kind: changeMod}}
	}
	if !rec.HasDB {
		var entries []changeEntry
		for name := range staged {
			entries = append(entries, changeEntry{trimExt(name), changeAdd})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].base < entries[j].base })
		return entries
	}
	stored, ok := collectionFiles([]byte(rec.DB.Content), rec.DB.Format)
	if !ok {
		return []changeEntry{{base: rec.Name, kind: changeMod}}
	}
	var entries []changeEntry
	for name, body := range staged {
		if prev, had := stored[name]; !had {
			entries = append(entries, changeEntry{trimExt(name), changeAdd})
		} else if prev != body {
			entries = append(entries, changeEntry{trimExt(name), changeMod})
		}
	}
	for name := range stored {
		if _, still := staged[name]; !still {
			entries = append(entries, changeEntry{trimExt(name), changeDel})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].base < entries[j].base })
	return entries
}

func changedSummary(rec sisyphus.Reconciliation, budget int) string {
	entries := changeEntries(rec)
	if len(entries) == 0 {
		return ""
	}
	th := theme.Cur()
	warn := lipgloss.NewStyle().Foreground(th.NotifWarning.GetForeground())
	var shown []string
	used := 0
	for _, e := range entries {
		plain := changePlain(e)
		if used > 0 && used+len(plain)+2 > max(budget, 20) {
			break
		}
		shown = append(shown, changeStyled(th, warn, e))
		used += len(plain) + 2
	}
	out := strings.Join(shown, th.Dim.Render(", "))
	if rest := len(entries) - len(shown); rest > 0 {
		out += th.Dim.Render(fmt.Sprintf(" +%d more", rest))
	}
	return out
}

func changePlain(e changeEntry) string {
	switch e.kind {
	case changeAdd:
		return "+" + e.base
	case changeDel:
		return "-" + e.base
	default:
		return e.base
	}
}

func changeStyled(th *theme.Theme, warn lipgloss.Style, e changeEntry) string {
	switch e.kind {
	case changeAdd:
		return th.Can.Render("+" + e.base)
	case changeDel:
		return th.Cant.Render("-" + e.base)
	default:
		return warn.Render(e.base)
	}
}

func collectionFiles(blob []byte, format string) (map[string]string, bool) {
	if format != "collection" {
		return nil, false
	}
	var files map[string]string
	if err := json.Unmarshal(blob, &files); err != nil {
		return nil, false
	}
	return files, true
}

func trimExt(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml"), ".json")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func humanBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f kB", float64(n)/1024)
}
