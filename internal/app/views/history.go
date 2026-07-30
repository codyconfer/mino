package views

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/list"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/signals"
)

const historyLimit = 50

func (k *Kit) History() vkdeck.View {
	rows := map[string]audit.AuditRow{}
	lst := vkdeck.NewItemList("history", k.menuCtx(),
		func() any { return k.recentRuns() },
		func(width int, fetched any) []list.Item {
			runs, _ := fetched.([]audit.AuditRow)
			rows = runIndex(runs)
			return historyItems(width, runs)
		},
	)
	lst.ReloadHint = "refresh"
	lst.OnOpen = func(string) error { return nil }
	lst.OnSelect = func(a *vkdeck.Model, key string) tea.Cmd {
		r, ok := rows[key]
		if !ok {
			return nil
		}
		return a.Push(k.historyRun(r))
	}
	return lst
}

func (k *Kit) recentRuns() []audit.AuditRow {
	st := k.d.App.Audit
	if st == nil {
		return nil
	}
	runs, err := st.RecentEntries(historyLimit)
	if err != nil {
		return nil
	}
	return runs
}

func runIndex(runs []audit.AuditRow) map[string]audit.AuditRow {
	out := make(map[string]audit.AuditRow, len(runs))
	for _, r := range runs {
		out[strconv.FormatInt(r.ID, 10)] = r
	}
	return out
}

func historyItems(width int, runs []audit.AuditRow) []list.Item {
	th := theme.Cur()
	if len(runs) == 0 {
		return []list.Item{{Block: th.Dim.Render("no recorded runs")}}
	}
	items := make([]list.Item, 0, len(runs))
	for _, r := range runs {
		label := th.Val.Render(fmt.Sprintf("#%-4d %-6s %s", r.ID, r.Kind, r.Name))
		meta := th.Dim.Render(r.Started.Format("01-02 15:04") + "  " + entryStatus(r))
		items = append(items, list.Item{
			Block:      layout.StackTight(label, layout.ScreenFrame(width).Fit(meta)),
			Key:        strconv.FormatInt(r.ID, 10),
			Selectable: true,
		})
	}
	return items
}

func entryStatus(r audit.AuditRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}

type historyRunView struct {
	*vkdeck.ItemList

	kit     *Kit
	row     audit.AuditRow
	ctx     [][2]string
	del     *keys.Map
	confirm *forms.Confirm
}

func (k *Kit) historyRun(r audit.AuditRow) vkdeck.View {
	title := r.Kind + ": " + r.Name
	if r.Kind == "flight" {
		title = "flight: " + r.Name
	}
	ctx := append(k.menuCtx(), [2]string{r.Kind, r.Name})
	id := r.ID
	lst := deck.NewResults(title, r.Name, ctx, func() []signals.Section {
		st := k.d.App.Audit
		if st == nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: fmt.Errorf("audit disabled")}}
		}
		secs, err := st.Sections(id)
		if err != nil {
			return []signals.Section{{Signal: r.Name, Title: r.Name, Err: err}}
		}
		return secs
	}, k.openDetail)
	return &historyRunView{
		ItemList: lst,
		kit:      k,
		row:      r,
		ctx:      ctx,
		del:      keys.NewMap(keymap.DeleteBinding()),
	}
}

func (v *historyRunView) Hints() [][2]string {
	return append(v.ItemList.Hints(), v.del.Hint(keymap.Delete))
}

func (v *historyRunView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return v.ItemList.Update(a, msg)
	}
	if v.confirm != nil {
		return v.answer(a, key)
	}
	if act, ok := v.del.Action(key.String()); ok && act == keymap.Delete {
		v.ask()
		return nil
	}
	return v.ItemList.Update(a, msg)
}

func (v *historyRunView) ask() {
	v.confirm = &forms.Confirm{
		Title:    fmt.Sprintf("delete run #%d?", v.row.ID),
		Message:  "This drops the recorded run and its items.",
		YesLabel: "Delete",
		NoLabel:  "Keep",
	}
}

func (v *historyRunView) answer(a *vkdeck.Model, key tea.KeyMsg) tea.Cmd {
	act, ok := keymap.ConfirmMap().Action(key.String())
	if !ok {
		return nil
	}
	switch v.confirm.Handle(act) {
	case forms.Submitted:
		yes := v.confirm.Yes
		v.confirm = nil
		if !yes {
			return nil
		}
		return v.remove(a)
	case forms.Cancelled:
		v.confirm = nil
	}
	return nil
}

func (v *historyRunView) remove(a *vkdeck.Model) tea.Cmd {
	if err := v.kit.d.App.Audit.Delete(v.row.ID); err != nil {
		return a.Push(vkdeck.NewMessage("delete failed", err.Error(), v.ctx))
	}
	return tea.Sequence(a.Pop(), reloadCmd())
}

func (v *historyRunView) Body(width, height int) string {
	body := v.ItemList.Body(width, height)
	if v.confirm == nil {
		return body
	}
	return v.confirm.Overlay(body, layout.NewFrame(layout.DialogWidth(width)))
}
