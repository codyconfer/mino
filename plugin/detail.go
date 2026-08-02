package plugin

import (
	"encoding/json"

	"github.com/codyconfer/viewkit/glyph"
)

type Chip struct {
	Label string
	Sev   glyph.Severity
}

type DetailSection struct {
	Title string            `json:"title"`
	Icon  string            `json:"icon,omitempty"`
	Rows  [][2]string       `json:"rows,omitempty"`
	Lines []string          `json:"lines,omitempty"`
	Body  string            `json:"body,omitempty"`
	Meta  map[string]string `json:"meta,omitempty"`
}

type ItemDetail struct {
	Kind     string            `json:"kind,omitempty"`
	Title    string            `json:"title"`
	URL      string            `json:"url,omitempty"`
	Chips    []Chip            `json:"chips,omitempty"`
	Rows     [][2]string       `json:"rows,omitempty"`
	Body     string            `json:"body,omitempty"`
	Sections []DetailSection   `json:"sections,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

var sevNames = map[glyph.Severity]string{
	glyph.SeverityNeutral:  "neutral",
	glyph.SeverityPositive: "positive",
	glyph.SeverityWarning:  "warning",
	glyph.SeverityNegative: "negative",
}

type wireChip struct {
	Label string `json:"label"`
	Sev   string `json:"sev,omitempty"`
}

func (c Chip) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireChip{Label: c.Label, Sev: sevNames[c.Sev]})
}

func (c *Chip) UnmarshalJSON(b []byte) error {
	var w wireChip
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	c.Label, c.Sev = w.Label, glyph.SeverityNeutral
	for sev, name := range sevNames {
		if name == w.Sev {
			c.Sev = sev
			break
		}
	}
	return nil
}
