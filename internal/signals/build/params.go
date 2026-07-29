package build

import "sort"

type ParamSpec struct {
	Key     string
	Desc    string
	Example string
}

var queryParams = map[string][]ParamSpec{
	"github": {
		{Key: "query", Desc: "GitHub search expression", Example: "is:open is:pr author:@me"},
		{Key: "project", Desc: "project board reference", Example: "owner/12"},
		{Key: "filter", Desc: "project board filter (needs project)", Example: "status:In Progress"},
		{Key: "title", Desc: "project board section title (needs project)", Example: "Sprint board"},
		{Key: "field", Desc: "project board field to group by (needs project)", Example: "Status"},
	},
	"gmail": {
		{Key: "query", Desc: "Gmail search expression", Example: "is:unread in:inbox newer_than:2d"},
		{Key: "max", Desc: "maximum messages to return", Example: "10"},
	},
	"calendar": {
		{Key: "calendar_id", Desc: "calendar to read", Example: "primary"},
		{Key: "window", Desc: "how far ahead to look", Example: "12h"},
		{Key: "max", Desc: "maximum events to return", Example: "20"},
	},
	"docs": {
		{Key: "recent", Desc: "how many recent documents to list", Example: "10"},
	},
	"slack": {
		{Key: "channel", Desc: "channel to read (required)", Example: "eng-standup"},
		{Key: "limit", Desc: "maximum messages to return", Example: "50"},
	},
}

func QueryParams(signal string) []ParamSpec {
	specs, ok := queryParams[signal]
	if !ok {
		return nil
	}
	out := make([]ParamSpec, len(specs))
	copy(out, specs)
	return out
}

func ParamSignals() []string {
	out := make([]string, 0, len(queryParams))
	for name := range queryParams {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

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
