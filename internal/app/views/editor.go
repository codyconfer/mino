package views

import (
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

const editorAdhocLabel = "ad-hoc"

type editorShell = vkdeck.Editor

type editorDoc interface {
	editorKind() string
	editorTitle() string
	editorCtx() [][2]string
	editorSavedName() string
	editorFields(prev map[string]any) []forms.Field
	editorSync() bool
	editorSummary() string
	editorValue() (any, error)
	editorRun() (string, func() []signals.Section, error)
	editorVerify(any) Finding
	editorPersist(any) (string, error)
	editorRemove() string
}

func newEditorShell(doc editorDoc, seed map[string]any) *editorShell {
	base := &editorAdapter{doc: doc}
	if out, ok := doc.(editorOutput); ok {
		return vkdeck.NewEditor(&editorOutputAdapter{editorAdapter: base, out: out}, editorKeys(), seed)
	}
	return vkdeck.NewEditor(base, editorKeys(), seed)
}

func editorKeys() vkdeck.EditorKeys {
	return vkdeck.EditorKeys{
		Map:      keymap.Form(keymap.BuilderBindings()...),
		Confirm:  keymap.ConfirmMap(),
		Run:      keymap.Run,
		Save:     keymap.Save,
		Validate: keymap.Validate,
		Preview:  keymap.Preview,
		Delete:   keymap.Delete,
		Focus:    keymap.Focus,
		Copy:     keymap.Copy,
		Write:    keymap.Write,
	}
}

type editorOutput interface {
	CopyOutput() (string, error)
	WriteOutput() (string, error)
}

type editorAdapter struct{ doc editorDoc }

type editorOutputAdapter struct {
	*editorAdapter
	out editorOutput
}

func (a *editorOutputAdapter) CopyOutput() (string, error)  { return a.out.CopyOutput() }
func (a *editorOutputAdapter) WriteOutput() (string, error) { return a.out.WriteOutput() }

func (a *editorAdapter) Kind() string         { return a.doc.editorKind() }
func (a *editorAdapter) Title() string        { return a.doc.editorTitle() }
func (a *editorAdapter) Context() [][2]string { return a.doc.editorCtx() }
func (a *editorAdapter) SavedName() string    { return a.doc.editorSavedName() }
func (a *editorAdapter) Sync() bool           { return a.doc.editorSync() }
func (a *editorAdapter) Summary() string      { return a.doc.editorSummary() }
func (a *editorAdapter) Remove() string       { return a.doc.editorRemove() }

func (a *editorAdapter) Fields(prev map[string]any) []forms.Field {
	return a.doc.editorFields(prev)
}

func (a *editorAdapter) PreviewLines() []string {
	val, err := a.doc.editorValue()
	if err != nil {
		return []string{theme.Cur().Cant.Render(err.Error())}
	}
	data, err := yaml.Marshal(val)
	if err != nil {
		return []string{theme.Cur().Cant.Render(err.Error())}
	}
	return layout.Lines(string(data))
}

func (a *editorAdapter) ValidateLines() ([]string, error) {
	val, err := a.doc.editorValue()
	if err != nil {
		return nil, err
	}
	f := a.doc.editorVerify(val)
	lines := []string{directiveFindingLine(f)}
	if f.Msg != "" {
		lines = append(lines, "    "+theme.Cur().Dim.Render(f.Msg))
	}
	if f.OK && !f.Warn && f.Msg == "" {
		lines = append(lines, "    "+theme.Cur().Dim.Render("no problems found"))
	}
	return lines, nil
}

func (a *editorAdapter) Run() (string, func() vkdeck.Results, error) {
	label, fetch, err := a.doc.editorRun()
	if err != nil {
		return "", nil, err
	}
	return label, func() vkdeck.Results {
		return render.SectionResults{Label: label, Sections: fetch()}
	}, nil
}

func (a *editorAdapter) Persist() (string, error) {
	val, err := a.doc.editorValue()
	if err != nil {
		return "", err
	}
	return a.doc.editorPersist(val)
}

func editorRenameNote(from, rel string) string {
	if rel == "" {
		return "\nrenamed from " + from + "."
	}
	return "\nrenamed from " + from + " in " + rel + "."
}
