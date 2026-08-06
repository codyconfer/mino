package gcx

import (
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

func newGcxCmd() *cobra.Command {
	parent := cmd.QueryCmd(SignalName, "Grafana Cloud IRM incidents and gcx status", bindQueryFlags)
	parent.AddCommand(newDeclareCmd(), newActivityCmd(), newLoginCmd())
	return parent
}

func bindQueryFlags(c *cobra.Command, p *map[string]string) {
	var view, stack, status string
	var limit int
	c.Flags().StringVar(&view, "view", "", "surface to read (incidents|status)")
	c.Flags().StringVar(&stack, "stack", "", "Grafana Cloud stack host")
	c.Flags().StringVar(&status, "status", "", "incident status filter (active|resolved|all)")
	c.Flags().IntVar(&limit, "limit", 0, "maximum incidents to return")
	c.PreRun = func(*cobra.Command, []string) {
		setParam(*p, "view", view)
		setParam(*p, "stack", stack)
		setParam(*p, "status", status)
		if limit > 0 {
			(*p)["limit"] = strconv.Itoa(limit)
		}
	}
}

func setParam(p map[string]string, key, value string) {
	if value != "" {
		p[key] = value
	}
}

func newDeclareCmd() *cobra.Command {
	var severity, summary, labels, stack string
	var drill bool
	c := &cobra.Command{
		Use:   "declare <title>",
		Short: "Declare an IRM incident (requires plugins.gcx.allow_write)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p := map[string]string{
				"title":    strings.Join(args, " "),
				"severity": severity,
				"summary":  summary,
				"labels":   labels,
				"stack":    stack,
			}
			if drill {
				p["drill"] = "true"
			}
			client, settings, err := actionClient(c.Context(), p)
			if err != nil {
				return err
			}
			started := time.Now()
			inc, err := client.CreateIncident(c.Context(), newIncidentFrom(p, settings))
			if err != nil {
				return err
			}
			sections := []plugin.Section{sectionFromIncidents([]incident{inc})}
			sections[0].Title = "declared incident"
			cmd.RecordAction("gcx declare", started, time.Now(), sections)
			return cmd.EmitSections(c, SignalName, sections)
		},
	}
	c.Flags().StringVar(&severity, "severity", "", "incident severity")
	c.Flags().StringVar(&summary, "summary", "", "incident summary")
	c.Flags().StringVar(&labels, "labels", "", "comma-separated labels")
	c.Flags().StringVar(&stack, "stack", "", "Grafana Cloud stack host")
	c.Flags().BoolVar(&drill, "drill", false, "declare as a drill")
	return c
}

func newActivityCmd() *cobra.Command {
	var incidentID, kind, stack string
	c := &cobra.Command{
		Use:   "activity <body>",
		Short: "Post an activity note to an IRM incident (requires plugins.gcx.allow_write)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return addActivity(c.Context(), map[string]string{
				"incident": incidentID,
				"body":     strings.Join(args, " "),
				"kind":     kind,
				"stack":    stack,
			})
		},
	}
	c.Flags().StringVar(&incidentID, "incident", "", "incident id")
	c.Flags().StringVar(&kind, "kind", "", "activity kind")
	c.Flags().StringVar(&stack, "stack", "", "Grafana Cloud stack host")
	return c
}

func newLoginCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "login",
		Short: "Seal a Grafana service account token, re-sealing an existing one with --force",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			h := hostFn()
			if h == nil {
				return errx.New("gcx: no mino host is available")
			}
			cfg := FromHost(h)
			if !force && Authed(cfg.Store, cfg.TokenEnv) {
				return errx.New("gcx: a token is already sealed").
					WithHint("re-seal it with `mino gcx login --force`")
			}
			return login(c.Context(), h, nil, c.ErrOrStderr())
		},
	}
	c.Flags().BoolVar(&force, "force", false, "replace an already sealed token")
	return c
}
