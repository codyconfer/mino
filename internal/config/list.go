package config

import "strings"

func ListValues(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			v := strings.TrimSpace(part)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
