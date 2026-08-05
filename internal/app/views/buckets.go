package views

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/plugin/ntr"
	"github.com/codyconfer/mino/internal/render"
)

func (k *Kit) bucketsEnabled() bool {
	home, _ := k.ntrHomeRole()
	return home != "" && plugin.SignalEnabled(ntr.SignalName)
}

func (k *Kit) NTRBuckets() vkdeck.View {
	home, role := k.ntrHomeRole()
	return ntr.NewBucketsView(home, role)
}

func (k *Kit) filePicker(t ntr.BucketTarget, sc keys.Scheme, dirty func()) vkdeck.View {
	home, role := k.ntrHomeRole()
	return ntr.NewBucketPicker(home, role, t, dirty, sc)
}

type resultsFileView struct {
	*deck.Results

	kit   *Kit
	ctx   []keys.Hint
	file  *keys.Map
	stale bool
}

func (k *Kit) withFile(lst *deck.Results, ctx []keys.Hint) vkdeck.View {
	if !k.bucketsEnabled() {
		return lst
	}
	return &resultsFileView{
		Results: lst,
		kit:     k,
		ctx:     ctx,
		file:    k.scope().Keys.MapFor(keymap.File),
	}
}

func (v *resultsFileView) Hints(scope *ui.Scope) []keys.Hint {
	return append(v.Results.Hints(scope), v.file.Hint(keymap.File))
}

func (v *resultsFileView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if act, ok := v.file.Action(m.String()); ok && act == keymap.File {
			return v.push(a)
		}
	case tea.WindowSizeMsg:
		cmd := v.Results.Update(a, msg)
		if v.stale {
			v.stale = false
			return tea.Batch(cmd, reloadCmd())
		}
		return cmd
	}
	return v.Results.Update(a, msg)
}

func (v *resultsFileView) push(a *vkdeck.Model) tea.Cmd {
	it, ok := v.Selected()
	if !ok {
		return nil
	}
	ref, ok := it.Payload.(render.ItemRef)
	if !ok {
		return nil
	}
	return a.Push(v.kit.filePicker(ntr.ItemTarget(ref.Signal, ref.Item),
		modelScope(a).Keys, func() { v.stale = true }))
}
