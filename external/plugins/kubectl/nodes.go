package kubectl

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

type nodeList struct {
	Items []nodeWire `json:"items"`
}

type nodeWire struct {
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Unschedulable bool `json:"unschedulable"`
	} `json:"spec"`
	Status struct {
		Conditions []nodeCondition `json:"conditions"`
	} `json:"status"`
}

type nodeCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

func fetchNodes(ctx context.Context, r runner, s scope, opts options) plugin.Section {
	raw, err := get(ctx, r, s.clusterScoped(), "nodes")
	if err != nil {
		return errSection(titleNodes, err)
	}
	sec, err := mapNodesJSON(raw, opts.Limit)
	if err != nil {
		return errSection(titleNodes, err)
	}
	return sec
}

func mapNodesJSON(raw []byte, limit int) (plugin.Section, error) {
	var list nodeList
	if err := json.Unmarshal(raw, &list); err != nil {
		return plugin.Section{}, errx.Wrap(err, "kubectl nodes")
	}

	items := make([]plugin.Item, 0, len(list.Items))
	for _, n := range list.Items {
		reasons, sev, at := nodeVerdict(n)
		if len(reasons) == 0 {
			continue
		}
		items = append(items, plugin.Item{
			Kind:      "node",
			Title:     n.Metadata.Name,
			Subtitle:  reasons[0],
			Body:      strings.Join(reasons, " · "),
			Timestamp: at,
			Meta: map[string]string{
				"node":     n.Metadata.Name,
				"reason":   reasons[0],
				"severity": sev,
			},
		})
	}
	sortItems(items)
	return plugin.Section{
		Signal: SignalName,
		Title:  titleNodes,
		Items:  truncate(items, limit),
		Meta:   map[string]string{"scanned": strconv.Itoa(len(list.Items))},
	}, nil
}

func nodeVerdict(n nodeWire) (reasons []string, severity string, at time.Time) {
	severity = sevWarning
	for _, c := range n.Status.Conditions {
		switch c.Type {
		case "Ready":
			if c.Status != "True" {
				reason := "NotReady"
				if c.Status == "Unknown" {
					reason = "Ready=Unknown"
				}
				if c.Reason != "" {
					reason += " (" + c.Reason + ")"
				}
				reasons = append([]string{reason}, reasons...)
				severity = sevCritical
				at = laterOf(at, c.LastTransitionTime)
			}
		case "MemoryPressure", "DiskPressure", "PIDPressure", "NetworkUnavailable":
			if c.Status == "True" {
				reasons = append(reasons, c.Type)
				at = laterOf(at, c.LastTransitionTime)
			}
		}
	}
	if n.Spec.Unschedulable {
		reasons = append(reasons, "cordoned")
	}
	if at.IsZero() {
		at = n.Metadata.CreationTimestamp
	}
	return reasons, severity, at
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
