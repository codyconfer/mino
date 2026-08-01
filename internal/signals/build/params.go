package build

import (
	"sort"

	"github.com/codyconfer/mino/internal/plugin"
)

type ParamSpec = plugin.ParamSpec

var githubSearchTerms = []string{
	"is:open", "is:closed", "is:pr", "is:issue", "is:draft",
	"author:@me", "assignee:@me", "review-requested:@me", "involves:@me",
	"archived:false", "sort:updated-desc",
}

func registerStockQueryParams() {
	plugin.RegisterQueryParams("github",
		ParamSpec{Key: "query", Desc: "GitHub search expression", Example: "is:open is:pr author:@me", Values: githubSearchTerms, Delim: " "},
		ParamSpec{Key: "project", Desc: "project board reference", Example: "owner/12"},
		ParamSpec{Key: "filter", Desc: "project board filter (needs project)", Example: "status:In Progress"},
		ParamSpec{Key: "title", Desc: "project board section title (needs project)", Example: "Sprint board"},
		ParamSpec{Key: "field", Desc: "project board field to group by (needs project)", Example: "Status"},
		ParamSpec{Key: "team", Desc: "org team whose members count as ours (needs project)", Example: "acme/platform"},
	)
}

func ParamKeys(signal string) []string {
	specs := QueryParams(signal)
	out := make([]string, 0, len(specs))
	for _, p := range specs {
		out = append(out, p.Key)
	}
	return out
}

func QueryParams(signal string) []ParamSpec { return plugin.QueryParams(signal) }

func ParamSignals() []string { return plugin.ParamSignals() }

func QueryableSignals() []string {
	var out []string
	for name, ok := range BuilderSignals() {
		if ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
