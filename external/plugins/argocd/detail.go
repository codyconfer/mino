package argocd

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

func (s *Signal) Detail(ctx context.Context, it plugin.Item) (plugin.ItemDetail, error) {
	name, appNamespace, err := s.refFromItem(it)
	if err != nil {
		return plugin.ItemDetail{}, err
	}

	app, err := s.client.Application(ctx, name, appNamespace)
	if err != nil {
		return plugin.ItemDetail{}, err
	}

	tree, treeErr := s.client.ResourceTree(ctx, name, appNamespace)

	var rev revisionMetadata
	var revOK bool
	if revision := strings.TrimSpace(app.Status.Sync.Revision); revision != "" && app.source().Chart == "" {
		if meta, err := s.client.RevisionMetadata(ctx, name, appNamespace, revision); err == nil {
			rev, revOK = meta, true
		}
	}

	return buildDetail(app, tree, treeErr, rev, revOK, s.cfg), nil
}

func (s *Signal) refFromItem(it plugin.Item) (name, appNamespace string, err error) {
	if n := strings.TrimSpace(it.Meta["app"]); n != "" {
		return n, strings.TrimSpace(it.Meta["app_namespace"]), nil
	}
	if n, ns, ok := parseAppURL(it.URL); ok {
		return n, ns, nil
	}
	if n := strings.TrimSpace(it.Title); n != "" {
		return n, s.cfg.AppNamespace, nil
	}
	return "", "", errx.Newf("argocd: cannot tell which application %q refers to", it.URL).
		WithHint("use an application URL such as https://<server>/applications/<app>")
}

func parseAppURL(raw string) (name, appNamespace string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p != "applications" {
			continue
		}
		switch len(parts) - i - 1 {
		case 1:
			return parts[i+1], "", true
		case 2:
			return parts[i+2], parts[i+1], true
		}
		return "", "", false
	}
	return "", "", false
}

func buildDetail(app application, tree resourceTree, treeErr error, rev revisionMetadata, revOK bool, cfg Config) plugin.ItemDetail {
	state := argoState(app)
	src := app.source()

	d := plugin.ItemDetail{
		Kind:  itemKindApp,
		Title: app.Metadata.Name,
		URL:   appURL(cfg.ServerURL, app.Metadata.Name, app.Metadata.Namespace),
		Chips: chipsFor(app),
		Rows:  detailRows(app, src),
		Body:  bodyOf(app),
		Meta:  map[string]string{"state": state},
	}
	if inProgress(state) {
		d.Meta["in_progress"] = "true"
	}

	if sec, ok := resourcesSection(app, tree, treeErr); ok {
		d.Sections = append(d.Sections, sec)
	}
	if sec, ok := historySection(app); ok {
		d.Sections = append(d.Sections, sec)
	}
	if sec, ok := operationSection(app); ok {
		d.Sections = append(d.Sections, sec)
	}
	if revOK {
		if sec, ok := commitSection(app, rev); ok {
			d.Sections = append(d.Sections, sec)
		}
	}
	if sec, ok := conditionsSection(app); ok {
		d.Sections = append(d.Sections, sec)
	}
	return d
}

func chipsFor(app application) []plugin.Chip {
	chips := make([]plugin.Chip, 0, 3)
	add := func(label string) {
		if label = strings.TrimSpace(label); label != "" {
			chips = append(chips, plugin.Chip{Label: label, Sev: severityFor(label)})
		}
	}
	add(app.Status.Health.Status)
	add(app.Status.Sync.Status)
	add(app.phase())
	return chips
}

func detailRows(app application, src applicationSrc) [][2]string {
	rows := make([][2]string, 0, 8)
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			rows = append(rows, [2]string{label, value})
		}
	}
	add("project", app.Spec.Project)
	add("cluster", app.cluster())
	add("namespace", app.Spec.Destination.Namespace)
	add("repo", src.RepoURL)
	add("path", src.Path)
	add("target", src.TargetRevision)
	add("revision", shortRev(app.Status.Sync.Revision, src.Chart))
	add("synced by", app.initiatedBy())
	if op := app.Status.OperationState; op != nil {
		add("synced", formatTime(op.FinishedAt))
	}
	return rows
}

func resourcesSection(app application, tree resourceTree, treeErr error) (plugin.DetailSection, bool) {
	if len(app.Status.Resources) == 0 && treeErr == nil {
		return plugin.DetailSection{}, false
	}

	nodeHealth := make(map[string]string, len(tree.Nodes))
	for _, n := range tree.Nodes {
		if n.Health != nil && n.Health.Status != "" {
			nodeHealth[n.Kind+"/"+n.Namespace+"/"+n.Name] = n.Health.Status
		}
	}

	sec := plugin.DetailSection{Title: "resources", Meta: map[string]string{"state_rows": "true"}}
	anyInProgress := false
	for i, r := range app.Status.Resources {
		if i >= maxDetailResourceRow {
			sec.Lines = append(sec.Lines,
				strconv.Itoa(len(app.Status.Resources)-maxDetailResourceRow)+" more resources not shown")
			break
		}
		state := resourceState(r, nodeHealth)
		if strings.EqualFold(state, "Progressing") {
			anyInProgress = true
		}
		sec.Rows = append(sec.Rows, [2]string{r.Kind + "/" + r.Name, state})
	}
	if treeErr != nil {
		sec.Lines = append(sec.Lines, "resource tree unavailable: "+treeErr.Error())
	}
	if anyInProgress {
		sec.Meta["in_progress"] = "true"
	}
	if len(sec.Rows) == 0 && len(sec.Lines) == 0 {
		return plugin.DetailSection{}, false
	}
	return sec, true
}

func resourceState(r resourceStatus, nodeHealth map[string]string) string {
	if h := nodeHealth[r.Kind+"/"+r.Namespace+"/"+r.Name]; h != "" {
		return h
	}
	if r.Health != nil && r.Health.Status != "" {
		return r.Health.Status
	}
	return r.Status
}

func historySection(app application) (plugin.DetailSection, bool) {
	if len(app.Status.History) == 0 {
		return plugin.DetailSection{}, false
	}
	sec := plugin.DetailSection{Title: "sync history"}
	chart := app.source().Chart
	for i := len(app.Status.History) - 1; i >= 0 && len(sec.Rows) < maxHistoryRows; i-- {
		h := app.Status.History[i]
		sec.Rows = append(sec.Rows, [2]string{shortRev(h.Revision, chart), formatTime(h.DeployedAt)})
	}
	return sec, len(sec.Rows) > 0
}

func operationSection(app application) (plugin.DetailSection, bool) {
	op := app.Status.OperationState
	if op == nil {
		return plugin.DetailSection{}, false
	}
	sec := plugin.DetailSection{Title: "last operation"}
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			sec.Rows = append(sec.Rows, [2]string{label, value})
		}
	}
	add("phase", op.Phase)
	add("started", formatTime(op.StartedAt))
	add("finished", formatTime(op.FinishedAt))
	if op.RetryCount > 0 {
		add("retries", strconv.Itoa(op.RetryCount))
	}
	add("initiated by", app.initiatedBy())
	if msg := strings.TrimSpace(op.Message); msg != "" {
		sec.Body = msg
	}
	if op.SyncResult != nil {
		for _, r := range op.SyncResult.Resources {
			if failedResource(r) {
				sec.Lines = append(sec.Lines, r.Kind+"/"+r.Name+": "+firstNonEmpty(r.Message, r.Status))
			}
		}
	}
	if len(sec.Rows) == 0 && len(sec.Lines) == 0 && sec.Body == "" {
		return plugin.DetailSection{}, false
	}
	return sec, true
}

func failedResource(r resourceResult) bool {
	if strings.EqualFold(r.Status, "SyncFailed") {
		return true
	}
	return strings.TrimSpace(r.Message) != "" && !strings.EqualFold(r.Status, "Synced")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func commitSection(app application, rev revisionMetadata) (plugin.DetailSection, bool) {
	sec := plugin.DetailSection{Title: "commit"}
	if author := strings.TrimSpace(rev.Author); author != "" {
		sec.Rows = append(sec.Rows, [2]string{"author", author})
	}
	if rev.Date != nil && !rev.Date.IsZero() {
		sec.Rows = append(sec.Rows, [2]string{"date", rev.Date.UTC().Format(time.RFC3339)})
	}
	if r := shortRev(app.Status.Sync.Revision, app.source().Chart); r != "" {
		sec.Rows = append(sec.Rows, [2]string{"revision", r})
	}
	sec.Body = strings.TrimSpace(rev.Message)
	if len(sec.Rows) == 0 && sec.Body == "" {
		return plugin.DetailSection{}, false
	}
	return sec, true
}

func conditionsSection(app application) (plugin.DetailSection, bool) {
	if len(app.Status.Conditions) == 0 {
		return plugin.DetailSection{}, false
	}
	sec := plugin.DetailSection{Title: "conditions"}
	for _, c := range app.Status.Conditions {
		sec.Rows = append(sec.Rows, [2]string{c.Type, strings.TrimSpace(c.Message)})
	}
	return sec, true
}
