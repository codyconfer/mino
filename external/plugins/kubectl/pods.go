package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const DefaultRestartThreshold = 5

const (
	sevCritical = "critical"
	sevWarning  = "warning"
)

type podList struct {
	Items []podWire `json:"items"`
}

type podWire struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string            `json:"phase"`
		Reason            string            `json:"reason"`
		Conditions        []podCondition    `json:"conditions"`
		ContainerStatuses []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

type objectMeta struct {
	Name              string    `json:"name"`
	Namespace         string    `json:"namespace"`
	CreationTimestamp time.Time `json:"creationTimestamp"`
}

type podCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type containerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int    `json:"restartCount"`
	State        struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
		Terminated *struct {
			Reason   string `json:"reason"`
			ExitCode int    `json:"exitCode"`
		} `json:"terminated"`
	} `json:"state"`
}

func fetchPods(ctx context.Context, r runner, s scope, opts options) plugin.Section {
	raw, err := get(ctx, r, s, "pods")
	if err != nil {
		return errSection(titlePods, err)
	}
	sec, err := mapPodsJSON(raw, opts.RestartThreshold, opts.Limit)
	if err != nil {
		return errSection(titlePods, err)
	}
	return sec
}

func mapPodsJSON(raw []byte, restartThreshold, limit int) (plugin.Section, error) {
	var list podList
	if err := json.Unmarshal(raw, &list); err != nil {
		return plugin.Section{}, errx.Wrap(err, "kubectl pods")
	}
	if restartThreshold <= 0 {
		restartThreshold = DefaultRestartThreshold
	}

	items := make([]plugin.Item, 0, len(list.Items))
	for _, p := range list.Items {
		reason, sev, ok := podVerdict(p, restartThreshold)
		if !ok {
			continue
		}
		restarts := podRestarts(p)
		items = append(items, plugin.Item{
			Kind:      "pod",
			Title:     p.Metadata.Namespace + "/" + p.Metadata.Name,
			Subtitle:  reason,
			Body:      podBody(p, restarts),
			Timestamp: p.Metadata.CreationTimestamp,
			Meta: map[string]string{
				"namespace": p.Metadata.Namespace,
				"pod":       p.Metadata.Name,
				"node":      p.Spec.NodeName,
				"phase":     p.Status.Phase,
				"reason":    reason,
				"restarts":  strconv.Itoa(restarts),
				"severity":  sev,
			},
		})
	}
	sortItems(items)
	return plugin.Section{
		Signal: SignalName,
		Title:  titlePods,
		Items:  truncate(items, limit),
		Meta:   map[string]string{"scanned": strconv.Itoa(len(list.Items))},
	}, nil
}

func podVerdict(p podWire, restartThreshold int) (reason, severity string, unhealthy bool) {
	switch p.Status.Phase {
	case "Succeeded":
		return "", "", false
	case "Failed":
		r := p.Status.Reason
		if r == "" {
			r = "Failed"
		}
		return r, sevCritical, true
	case "Pending":
		if r := waitingReason(p); r != "" {
			return r, sevWarning, true
		}
		if startingUp(p) {
			return "", "", false
		}
		return "Pending", sevWarning, true
	}

	if r := waitingReason(p); r != "" {
		return r, sevCritical, true
	}
	if !podReady(p) {
		if startingUp(p) {
			return "", "", false
		}
		return "NotReady", sevWarning, true
	}
	if podRestarts(p) >= restartThreshold {
		return "Restarting", sevWarning, true
	}
	return "", "", false
}

func waitingReason(p podWire) string {
	for _, c := range p.Status.ContainerStatuses {
		w := c.State.Waiting
		if w == nil || w.Reason == "" {
			continue
		}
		switch w.Reason {
		case "ContainerCreating", "PodInitializing":
			continue
		}
		return w.Reason
	}
	return ""
}

func startingUp(p podWire) bool {
	for _, c := range p.Status.ContainerStatuses {
		if w := c.State.Waiting; w != nil {
			switch w.Reason {
			case "ContainerCreating", "PodInitializing":
				return true
			}
		}
	}
	return false
}

func podReady(p podWire) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

func podRestarts(p podWire) int {
	total := 0
	for _, c := range p.Status.ContainerStatuses {
		total += c.RestartCount
	}
	return total
}

func podBody(p podWire, restarts int) string {
	parts := []string{p.Status.Phase}
	ready, total := 0, len(p.Status.ContainerStatuses)
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
	}
	if total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d ready", ready, total))
	}
	if restarts > 0 {
		parts = append(parts, fmt.Sprintf("%d restarts", restarts))
	}
	if p.Spec.NodeName != "" {
		parts = append(parts, "on "+p.Spec.NodeName)
	}
	return strings.Join(parts, " · ")
}

func sortItems(items []plugin.Item) {
	rank := func(it plugin.Item) int {
		if it.Meta["severity"] == sevCritical {
			return 0
		}
		return 1
	}
	sort.SliceStable(items, func(i, j int) bool {
		if ri, rj := rank(items[i]), rank(items[j]); ri != rj {
			return ri < rj
		}
		if !items[i].Timestamp.Equal(items[j].Timestamp) {
			return items[i].Timestamp.After(items[j].Timestamp)
		}
		return items[i].Title < items[j].Title
	})
}

func truncate(items []plugin.Item, limit int) []plugin.Item {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func errSection(title string, err error) plugin.Section {
	return plugin.Section{Signal: SignalName, Title: title, Err: err}
}
