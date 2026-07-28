package signals

import (
	"strings"

	"github.com/codyconfer/viewkit/glyph"
)

func ClassifyKind(kind string) glyph.Severity {
	switch strings.ToLower(kind) {
	case "mention", "review-requested", "review_requested", "assigned", "alert", "incident", "warn", "warning":
		return glyph.SeverityWarning
	case "merged", "approved", "completed", "resolved", "success", "closed":
		return glyph.SeverityPositive
	case "error", "failed", "failure":
		return glyph.SeverityNegative
	default:
		return glyph.SeverityNeutral
	}
}
