package render

import (
	"github.com/codyconfer/munin/internal/signals"
)

func cleanRef(ref ItemRef) ItemRef {
	ref.Signal = signals.CleanLine(ref.Signal)
	ref.Item = signals.CleanItem(ref.Item)
	ref.Meta = signals.CleanMeta(ref.Meta)
	return ref
}
