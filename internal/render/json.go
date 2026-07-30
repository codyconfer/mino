package render

import (
	"encoding/json"
	"io"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

type JSONRenderer struct{}

type wireSection struct {
	Signal string            `json:"signal"`
	Title  string            `json:"title"`
	Items  []signals.Item    `json:"items"`
	Meta   map[string]string `json:"meta,omitempty"`
	Error  string            `json:"error,omitempty"`
}

func (r *JSONRenderer) Render(w io.Writer, sections []signals.Section) error {
	wire := make([]wireSection, 0, len(sections))
	for _, s := range sections {
		s = signals.CleanSection(s)
		items := s.Items
		if items == nil {
			items = []signals.Item{}
		}
		wire = append(wire, wireSection{
			Signal: s.Signal,
			Title:  s.Title,
			Items:  items,
			Meta:   s.Meta,
			Error:  signals.CleanLine(s.ErrString()),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wire); err != nil {
		return errs.Wrap(errs.KindInternal, err, "encode json output")
	}
	return nil
}
