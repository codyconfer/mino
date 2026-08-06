package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/signals"
)

const detailCacheNS = "gitlab:detail"

func (p CachePolicy) reads() bool  { return p.Read }
func (p CachePolicy) writes() bool { return p.Write && p.TTL > 0 }

type Ref struct {
	Project string
	Kind    Kind
	ID      int64
}

func (r Ref) String() string {
	switch r.Kind {
	case KindIssue:
		return fmt.Sprintf("%s#%d", r.Project, r.ID)
	case KindPipeline:
		return fmt.Sprintf("%s@pipeline/%d", r.Project, r.ID)
	}
	return fmt.Sprintf("%s!%d", r.Project, r.ID)
}

func (r Ref) path(suffix string) string {
	base := "projects/" + encodeProject(r.Project) + "/" + surfacePath(r.Kind) + "/" +
		strconv.FormatInt(r.ID, 10)
	if suffix == "" {
		return base
	}
	return base + "/" + strings.TrimLeft(suffix, "/")
}

var refKinds = map[string]Kind{
	"merge_requests": KindMR,
	"issues":         KindIssue,
	"pipelines":      KindPipeline,
}

const refHint = "use a URL like https://gitlab.com/group/project/-/merge_requests/42"

// ParseRef splits on the /-/ separator rather than counting path segments, which is what
// makes arbitrarily nested subgroups work and is why GitLab introduced /-/ in the first
// place.
func ParseRef(rawURL string) (Ref, error) {
	raw := strings.TrimSpace(rawURL)
	if raw == "" {
		return Ref{}, errs.New(errs.KindUsage, "gitlab: no item URL").WithHint("%s", refHint)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Ref{}, errs.Wrapf(errs.KindUsage, err, "gitlab: %q is not a URL", rawURL).WithHint("%s", refHint)
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return Ref{}, errs.Newf(errs.KindUsage, "gitlab: %q has no path", rawURL).WithHint("%s", refHint)
	}

	project, rest, ok := splitRefPath(path)
	if !ok {
		return Ref{}, unsupportedRef(rawURL, path)
	}
	kind, ok := refKinds[rest[0]]
	if !ok {
		return Ref{}, unsupportedRef(rawURL, path)
	}
	if len(rest) < 2 {
		return Ref{}, errs.Newf(errs.KindUsage, "gitlab: %q names no %s", rawURL, rest[0]).WithHint("%s", refHint)
	}
	id, err := strconv.ParseInt(rest[1], 10, 64)
	if err != nil || id <= 0 {
		return Ref{}, errs.Newf(errs.KindUsage, "gitlab: %q is not a %s number in %q", rest[1], rest[0], rawURL).
			WithHint("%s", refHint)
	}
	if project == "" {
		return Ref{}, errs.Newf(errs.KindUsage, "gitlab: %q names no project", rawURL).WithHint("%s", refHint)
	}
	return Ref{Project: project, Kind: kind, ID: id}, nil
}

func splitRefPath(path string) (project string, rest []string, ok bool) {
	if strings.HasPrefix(path, "-/") {
		return "", strings.Split(strings.Trim(path[2:], "/"), "/"), true
	}
	if i := strings.Index(path, "/-/"); i >= 0 {
		return path[:i], strings.Split(strings.Trim(path[i+3:], "/"), "/"), true
	}
	segs := strings.Split(path, "/")
	for i := len(segs) - 2; i > 0; i-- {
		if _, known := refKinds[segs[i]]; known {
			return strings.Join(segs[:i], "/"), segs[i:], true
		}
	}
	return "", nil, false
}

func unsupportedRef(rawURL, path string) error {
	if strings.Contains(path, "/-/jobs/") {
		return errs.Newf(errs.KindUsage, "gitlab: %q is a job, not a pipeline", rawURL).
			WithHint("open the pipeline the job belongs to, e.g. .../-/pipelines/99")
	}
	return errs.Newf(errs.KindUsage, "gitlab: %q is not a merge request, issue or pipeline URL", rawURL).
		WithHint("%s", refHint)
}

func (s *Signal) Detail(ctx context.Context, it signals.Item) (signals.ItemDetail, error) {
	ref, err := ParseRef(it.URL)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	// A running pipeline's state churns, so a cached one is worse than none. The github
	// signal makes the same call for Actions runs.
	if ref.Kind == KindPipeline {
		return s.pipelineDetail(ctx, ref, it)
	}
	node, err := s.loadDetail(ctx, ref)
	if err != nil {
		return signals.ItemDetail{}, err
	}
	if ref.Kind == KindIssue {
		return node.issueDetail(ref, it), nil
	}
	return node.mergeRequestDetail(ref, it), nil
}

func (s *Signal) loadDetail(ctx context.Context, ref Ref) (*detailNode, error) {
	key := ref.String()
	if s.detail != nil && s.policy.reads() {
		if raw, ok := s.detail.Get(ctx, detailCacheNS, key); ok {
			var node detailNode
			if err := json.Unmarshal([]byte(raw), &node); err == nil {
				log.Debugf("gitlab: detail cache hit %s", key)
				return &node, nil
			}
			log.Debugf("gitlab: discarding unreadable detail cache entry %s", key)
		}
	}
	node, err := s.requestDetail(ctx, ref)
	if err != nil {
		return nil, err
	}
	if s.detail != nil && s.policy.writes() {
		if raw, err := json.Marshal(node); err != nil {
			log.Debugf("gitlab: detail cache encode failed: %v", err)
		} else {
			s.detail.Put(ctx, detailCacheNS, key, string(raw), timeNow().Add(s.policy.TTL))
		}
	}
	return node, nil
}

func (s *Signal) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	p, err := s.backend.Get(ctx, path, q)
	if err != nil {
		return nil, err
	}
	return p.Body, nil
}

func chip(label string, sev glyphSeverity) signals.Chip {
	return signals.Chip{Label: label, Sev: sev}
}

func row(rows [][2]string, key, value string) [][2]string {
	if value == "" {
		return rows
	}
	return append(rows, [2]string{key, value})
}

func relTime(iso string) string {
	t := parseTime(iso)
	if t.IsZero() {
		return ""
	}
	d := timeNow().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
