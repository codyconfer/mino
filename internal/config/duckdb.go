package config

import "strings"

var duckDBDatabases = []string{"audit", "config", "state", "tokens"}

func DuckDBDatabases() []string { return append([]string(nil), duckDBDatabases...) }

func ValidDuckDBDatabase(name string) bool {
	for _, candidate := range duckDBDatabases {
		if name == candidate {
			return true
		}
	}
	return false
}

func DuckDBReadOnly(sql string) bool {
	query := strings.TrimSpace(sql)
	query = strings.TrimSpace(strings.TrimSuffix(query, ";"))
	if query == "" || strings.Contains(query, ";") {
		return false
	}
	first, _, _ := strings.Cut(strings.ToLower(query), " ")
	first = strings.TrimSpace(first)
	for _, allowed := range []string{"select", "with", "pragma", "describe", "show"} {
		if first == allowed || strings.HasPrefix(first, allowed+"\n") || strings.HasPrefix(first, allowed+"\t") {
			return true
		}
	}
	return false
}
