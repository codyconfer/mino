package gitlab

import (
	"encoding/json"
	"strings"
	"time"
)

var timeNow = time.Now

func countTopLevel(body []byte) int {
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0
	}
	return len(rows)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t
}

func joinNames(list []string) string { return strings.Join(list, ",") }

func atNames(list []string) string {
	if len(list) == 0 {
		return ""
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != "" {
			out = append(out, "@"+s)
		}
	}
	return strings.Join(out, ",")
}

func putIf(meta map[string]string, key, value string) {
	if value != "" {
		meta[key] = value
	}
}
