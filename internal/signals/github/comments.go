package github

import (
	"slices"
	"strings"
	"time"
)

const commentWindow = 5

var botLoginSuffixes = []string{"[bot]", "-bot", "_bot", ".bot"}

func lastResponder(n searchNode) (string, time.Time) {
	for _, c := range slices.Backward(n.Comments.Nodes) {
		a := c.Author
		if a == nil || a.Login == "" {
			continue
		}
		if isBotLogin(a.Login, a.TypeName) {
			continue
		}
		return a.Login, parseTime(c.CreatedAt)
	}
	if n.Author != nil {
		return n.Author.Login, parseTime(n.CreatedAt)
	}
	return "", time.Time{}
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

func isBotLogin(login, typeName string) bool {
	if strings.EqualFold(typeName, "Bot") {
		return true
	}
	lower := strings.ToLower(login)
	for _, suffix := range botLoginSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
