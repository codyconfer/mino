package render

import (
	"io"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Format string

const (
	FormatTerminal Format = "terminal"
	FormatJSON     Format = "json"
)

type Renderer interface {
	Render(w io.Writer, sections []signals.Section) error
}

func New(f Format, root string) (Renderer, error) {
	switch f {
	case FormatTerminal, "":
		return &TerminalRenderer{Root: root}, nil
	case FormatJSON:
		return &JSONRenderer{}, nil
	default:
		return nil, errs.Newf(errs.KindUsage, "unknown output format %q", f).WithHint("want terminal or json")
	}
}
