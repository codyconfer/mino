package github

import (
	"slices"
	"strings"
)

const commentWindow = 5

var botLoginSuffixes = []string{"[bot]", "-bot", "_bot", ".bot"}

func lastResponder(n searchNode) string {
	for _, c := range slices.Backward(n.Comments.Nodes) {
		a := c.Author
		if a == nil || a.Login == "" {
			continue
		}
		if isBotLogin(a.Login, a.TypeName) {
			continue
		}
		return a.Login
	}
	if n.Author != nil {
		return n.Author.Login
	}
	return ""
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
