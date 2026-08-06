package argocd

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	stateFailed       = "failed"
	stateDegraded     = "degraded"
	stateMissing      = "missing"
	stateInProgress   = "in progress"
	stateProgressing  = "progressing"
	stateOutOfSync    = "out of sync"
	stateSuspended    = "suspended"
	stateSynced       = "synced"
	stateUnknown      = "unknown"
	itemKindApp       = "application"
	shortRevisionSize = 7
)

var stateRank = map[string]int{
	stateFailed:      0,
	stateDegraded:    1,
	stateMissing:     2,
	stateOutOfSync:   3,
	stateInProgress:  4,
	stateProgressing: 5,
	stateSuspended:   6,
	stateUnknown:     7,
	stateSynced:      8,
}

func MapApplicationsJSON(raw []byte, cfg Config) ([]plugin.Section, error) {
	var list applicationList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errx.Wrap(err, "argocd: decoding the application list")
	}
	return sectionsFrom(list.Items, cfg), nil
}

func sectionsFrom(apps []application, cfg Config) []plugin.Section {
	items := make([]plugin.Item, 0, len(apps))
	for _, app := range apps {
		if !keep(app, cfg) {
			continue
		}
		items = append(items, applicationToItem(app, cfg))
	}
	sortItems(items)

	total := len(items)
	dropped := 0
	if cfg.Max > 0 && total > cfg.Max {
		items = items[:cfg.Max]
		dropped = total - cfg.Max
	}

	if cfg.GroupBy == groupByNone {
		return []plugin.Section{sectionOf(SignalName, items, total, dropped, cfg)}
	}
	return groupedSections(items, total, dropped, cfg)
}

func groupedSections(items []plugin.Item, total, dropped int, cfg Config) []plugin.Section {
	metaKey := "project"
	if cfg.GroupBy == groupByCluster {
		metaKey = "cluster"
	}
	order := make([]string, 0, 8)
	groups := make(map[string][]plugin.Item, 8)
	for _, it := range items {
		key := it.Meta[metaKey]
		if key == "" {
			key = stateUnknown
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], it)
	}
	if len(order) == 0 {
		return []plugin.Section{sectionOf(SignalName, nil, total, dropped, cfg)}
	}
	out := make([]plugin.Section, 0, len(order))
	for i, key := range order {
		drop := 0
		if i == len(order)-1 {
			drop = dropped
		}
		out = append(out, sectionOf(SignalName+" · "+key, groups[key], len(groups[key])+drop, drop, cfg))
	}
	return out
}

func sectionOf(title string, items []plugin.Item, total, dropped int, cfg Config) plugin.Section {
	meta := map[string]string{
		"shown":  strconv.Itoa(len(items)),
		"total":  strconv.Itoa(total),
		"server": serverHost(cfg.ServerURL),
	}
	if dropped > 0 {
		meta[plugin.MetaMore] = strconv.Itoa(dropped)
		meta[plugin.MetaTruncated] = "true"
	}
	return plugin.Section{Signal: SignalName, Title: title, Items: items, Meta: meta}
}

func keep(app application, cfg Config) bool {
	if len(cfg.Namespaces) > 0 && !contains(cfg.Namespaces, app.Spec.Destination.Namespace) {
		return false
	}
	if cfg.OnlyUnhealthy && argoState(app) == stateSynced {
		return false
	}
	return true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func argoState(app application) string {
	health := app.Status.Health.Status
	sync := app.Status.Sync.Status
	switch app.phase() {
	case "Error", "Failed":
		return stateFailed
	}
	switch health {
	case "Degraded":
		return stateDegraded
	case "Missing":
		return stateMissing
	}
	switch app.phase() {
	case "Running", "Terminating":
		return stateInProgress
	}
	if health == "Progressing" {
		return stateProgressing
	}
	if sync == "OutOfSync" {
		return stateOutOfSync
	}
	if health == "Suspended" {
		return stateSuspended
	}
	if sync == "Synced" && health == "Healthy" {
		return stateSynced
	}
	return stateUnknown
}

func inProgress(state string) bool {
	return state == stateInProgress || state == stateProgressing
}

func severityFor(state string) glyph.Severity {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case stateSynced, "healthy", "succeeded":
		return glyph.SeverityPositive
	case stateFailed, stateDegraded, stateMissing, "error":
		return glyph.SeverityNegative
	case stateInProgress, stateProgressing, stateOutOfSync, "outofsync", "running", "terminating":
		return glyph.SeverityWarning
	default:
		return glyph.SeverityNeutral
	}
}

func applicationToItem(app application, cfg Config) plugin.Item {
	state := argoState(app)
	src := app.source()
	meta := map[string]string{
		"state":           state,
		"app":             app.Metadata.Name,
		"app_namespace":   app.Metadata.Namespace,
		"project":         app.Spec.Project,
		"cluster":         app.cluster(),
		"namespace":       app.Spec.Destination.Namespace,
		"sync":            app.Status.Sync.Status,
		"health":          app.Status.Health.Status,
		"phase":           app.phase(),
		"revision":        app.Status.Sync.Revision,
		"revision_short":  shortRev(app.Status.Sync.Revision, src.Chart),
		"repo":            src.RepoURL,
		"path":            src.Path,
		"target_revision": src.TargetRevision,
		"initiated_by":    app.initiatedBy(),
	}
	if op := app.Status.OperationState; op != nil {
		meta["sync_started"] = formatTime(op.StartedAt)
		meta["sync_finished"] = formatTime(op.FinishedAt)
	}
	if inProgress(state) {
		meta["in_progress"] = "true"
	}
	for k, v := range meta {
		if v == "" {
			delete(meta, k)
		}
	}
	return plugin.Item{
		Kind:      itemKindApp,
		Title:     app.Metadata.Name,
		Subtitle:  subtitleOf(app),
		Body:      bodyOf(app),
		URL:       appURL(cfg.ServerURL, app.Metadata.Name, app.Metadata.Namespace),
		Timestamp: timestampOf(app),
		Meta:      meta,
	}
}

func subtitleOf(app application) string {
	scope := app.cluster()
	if ns := app.Spec.Destination.Namespace; ns != "" {
		if scope == "" {
			scope = ns
		} else {
			scope += "/" + ns
		}
	}
	project := app.Spec.Project
	switch {
	case project == "" && scope == "":
		return ""
	case project == "":
		return scope
	case scope == "":
		return project
	}
	return project + " · " + scope
}

func bodyOf(app application) string {
	if msg := strings.TrimSpace(app.Status.Health.Message); msg != "" {
		return msg
	}
	for _, cond := range app.Status.Conditions {
		if msg := strings.TrimSpace(cond.Message); msg != "" {
			return msg
		}
	}
	if op := app.Status.OperationState; op != nil {
		return strings.TrimSpace(op.Message)
	}
	return ""
}

func timestampOf(app application) time.Time {
	var newest time.Time
	consider := func(t *time.Time) {
		if t != nil && t.After(newest) {
			newest = *t
		}
	}
	if op := app.Status.OperationState; op != nil {
		consider(op.FinishedAt)
		consider(op.StartedAt)
	}
	if h := app.latestHistory(); h != nil {
		consider(h.DeployedAt)
	}
	consider(app.Status.ReconciledAt)
	return newest
}

func shortRev(revision, chart string) string {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return ""
	}
	if chart != "" || len(revision) <= shortRevisionSize {
		return revision
	}
	if !isHex(revision) {
		return revision
	}
	return revision[:shortRevisionSize]
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func formatTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func sortItems(items []plugin.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := rankOf(items[i]), rankOf(items[j])
		if ri != rj {
			return ri < rj
		}
		ti, tj := items[i].Timestamp, items[j].Timestamp
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return items[i].Title < items[j].Title
	})
}

func rankOf(it plugin.Item) int {
	if r, ok := stateRank[it.Meta["state"]]; ok {
		return r
	}
	return stateRank[stateUnknown]
}
