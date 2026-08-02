package signals

import (
	"strings"

	"github.com/codyconfer/viewkit/glyph"
)

func ClassifyItem(it Item) glyph.Severity {
	if strings.EqualFold(it.Kind, "workflow") {
		if conclusion := it.Meta["conclusion"]; conclusion != "" {
			return ClassifyKind(conclusion)
		}
		return ClassifyKind(it.Meta["status"])
	}
	isPR := strings.EqualFold(it.Kind, "pr")
	switch strings.ToLower(it.Meta["state"]) {
	case "merged":
		return glyph.SeverityPositive
	case "closed":
		if isPR {
			return glyph.SeverityNegative
		}
		return glyph.SeverityPositive
	case "open":
		return glyph.SeverityNeutral
	}
	return ClassifyKind(it.Kind)
}

func ClassifyState(state string) glyph.Severity {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(state), " ", "_")) {
	case "SUCCESS":
		return glyph.SeverityPositive
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE":
		return glyph.SeverityNegative
	case "IN_PROGRESS", "QUEUED", "PENDING", "WAITING", "REQUESTED", "ACTION_REQUIRED":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}

func ClassifyKind(kind string) glyph.Severity {
	switch strings.ToLower(kind) {
	case "mention", "review-requested", "review_requested", "assigned", "alert", "incident", "warn", "warning", "queued", "in_progress", "pending", "waiting", "requested":
		return glyph.SeverityWarning
	case "merged", "approved", "completed", "resolved", "success", "closed":
		return glyph.SeverityPositive
	case "error", "failed", "failure":
		return glyph.SeverityNegative
	default:
		return glyph.SeverityNeutral
	}
}
