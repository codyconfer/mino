package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const workloadResources = "deployments,statefulsets,daemonsets"

type workloadList struct {
	Items []workloadWire `json:"items"`
}

type workloadWire struct {
	Kind     string     `json:"kind"`
	Metadata objectMeta `json:"metadata"`
	Spec     struct {
		Replicas *int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		Replicas          int `json:"replicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		UpdatedReplicas   int `json:"updatedReplicas"`
		AvailableReplicas int `json:"availableReplicas"`

		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		UpdatedNumberScheduled int `json:"updatedNumberScheduled"`
	} `json:"status"`
}

func fetchWorkloads(ctx context.Context, r runner, s scope, opts options) plugin.Section {
	raw, err := get(ctx, r, s, workloadResources)
	if err != nil {
		return errSection(titleWorkloads, err)
	}
	sec, err := mapWorkloadsJSON(raw, opts.Limit)
	if err != nil {
		return errSection(titleWorkloads, err)
	}
	return sec
}

func mapWorkloadsJSON(raw []byte, limit int) (plugin.Section, error) {
	var list workloadList
	if err := json.Unmarshal(raw, &list); err != nil {
		return plugin.Section{}, errx.Wrap(err, "kubectl workloads")
	}

	items := make([]plugin.Item, 0, len(list.Items))
	for _, w := range list.Items {
		desired, ready, updated := w.counts()
		if desired == 0 {
			continue
		}
		var reason, sev string
		switch {
		case ready == 0:
			reason, sev = "no ready replicas", sevCritical
		case ready < desired:
			reason, sev = "degraded", sevWarning
		case updated < desired:
			reason, sev = "rollout in progress", sevWarning
		default:
			continue
		}
		kind := w.Kind
		if kind == "" {
			kind = "Workload"
		}
		items = append(items, plugin.Item{
			Kind:      "workload",
			Title:     w.Metadata.Namespace + "/" + w.Metadata.Name,
			Subtitle:  fmt.Sprintf("%s · %s", strings.ToLower(kind), reason),
			Body:      fmt.Sprintf("%d/%d ready · %d/%d updated", ready, desired, updated, desired),
			Timestamp: w.Metadata.CreationTimestamp,
			Meta: map[string]string{
				"namespace": w.Metadata.Namespace,
				"workload":  w.Metadata.Name,
				"kind":      kind,
				"reason":    reason,
				"desired":   strconv.Itoa(desired),
				"ready":     strconv.Itoa(ready),
				"severity":  sev,
			},
		})
	}
	sortItems(items)
	return plugin.Section{
		Signal: SignalName,
		Title:  titleWorkloads,
		Items:  truncate(items, limit),
		Meta:   map[string]string{"scanned": strconv.Itoa(len(list.Items))},
	}, nil
}

func (w workloadWire) counts() (desired, ready, updated int) {
	if w.Kind == "DaemonSet" {
		return w.Status.DesiredNumberScheduled, w.Status.NumberReady, w.Status.UpdatedNumberScheduled
	}
	desired = w.Status.Replicas
	if w.Spec.Replicas != nil {
		desired = *w.Spec.Replicas
	}
	return desired, w.Status.ReadyReplicas, w.Status.UpdatedReplicas
}
