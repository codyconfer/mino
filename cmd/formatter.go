package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/viewkit/clipboard"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

type formatterFlags struct {
	name    string
	copyOut bool
	out     string
}

func (ff *formatterFlags) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&ff.name, "formatter", "F", "",
		"render results through this formatter instead of the normal output")
	f.BoolVar(&ff.copyOut, "copy", false, "copy the formatted report to the clipboard")
	f.StringVar(&ff.out, "out", "", "write the formatted report to this file")
	bindFlagCompletion(cmd, "formatter", completeFormatterNames)
}

func (ff *formatterFlags) bindSinks(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVar(&ff.copyOut, "copy", false, "copy the formatted report to the clipboard")
	f.StringVar(&ff.out, "out", "", "write the formatted report to this file")
}

type outputRequest struct {
	raw      string
	format   render.Format
	explicit bool
}

func (o outputRequest) rendersFormatter() bool { return o.format == render.FormatTerminal }

func requestedOutput(cmd *cobra.Command) (outputRequest, error) {
	o := outputRequest{raw: shared.Cfg.Output, explicit: outputFlagChanged(cmd)}
	if o.raw == "" {
		o.raw = string(render.FormatTerminal)
	}
	o.format = render.Format(o.raw)
	if _, err := render.New(o.format, ""); err != nil {
		return outputRequest{}, err
	}
	return o, nil
}

func outputFlagChanged(cmd *cobra.Command) bool {
	if cmd != nil {
		if f := cmd.Flags().Lookup("output"); f != nil && f.Changed {
			return true
		}
	}
	return flagOutput != ""
}

func requireFormatterOutput(cmd *cobra.Command, name string) error {
	o, err := requestedOutput(cmd)
	if err != nil {
		return err
	}
	if o.explicit && !o.rendersFormatter() {
		return errs.Newf(errs.KindUsage, "formatter %q renders text, which --output %s cannot carry", name, o.raw).
			WithHint("drop --output %s to get the report, or drop the formatter to get %s results", o.raw, o.raw)
	}
	return nil
}

func (ff *formatterFlags) resolve(cmd *cobra.Command, fallback string) (runOpts, error) {
	o, err := requestedOutput(cmd)
	if err != nil {
		return runOpts{}, err
	}
	name := ff.name
	fromFlag := name != ""
	if name == "" {
		name = fallback
	}
	if name == "" {
		if ff.copyOut || ff.out != "" {
			return runOpts{}, errs.New(errs.KindUsage, "--copy and --out only apply with a formatter").
				WithHint("add --formatter <name>, or set `formatter:` on the query or flight")
		}
		return runOpts{}, nil
	}
	if o.explicit && !o.rendersFormatter() {
		if fromFlag {
			return runOpts{}, requireFormatterOutput(cmd, name)
		}
		if ff.copyOut || ff.out != "" {
			return runOpts{}, errs.Newf(errs.KindUsage, "--output %s ignores the configured formatter %q, so --copy/--out have nothing to write", o.raw, name).
				WithHint("drop --output %s, or pass --formatter %s to force the report", o.raw, name)
		}
		verbosef("--output %s overrides the configured formatter %q", o.raw, name)
		return runOpts{}, nil
	}
	fd, err := lookupFormatter(name)
	if err != nil {
		return runOpts{}, err
	}
	return runOpts{formatter: fd, active: true, copyOut: ff.copyOut, out: ff.out}, nil
}

func lookupFormatter(name string) (config.FormatterDef, error) {
	fd, ok := shared.Directives.Formatters[name]
	if !ok {
		return config.FormatterDef{}, errs.Newf(errs.KindUsage, "no formatter named %q%s", name, availableFormatterSuffix()).
			WithHint("run `munin formatter` to list available formatters")
	}
	if !access().FormatterVisible(name) {
		return config.FormatterDef{}, notInRoleError("formatter", name)
	}
	return fd, nil
}

func availableFormatterSuffix() string {
	names := visibleFormatterNames()
	if len(names) == 0 {
		return " (no formatters visible)"
	}
	return " (available: " + strings.Join(names, ", ") + ")"
}

func copyFunc(o runOpts) func(string) error {
	if !o.copyOut {
		return nil
	}
	return clipboard.Copy
}

func newFormatterCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "formatter [name]",
		Short: "Inspect and run saved formatters (templated reports over results)",
		Long: "A formatter is a `type: formatter` document holding a Go text/template that\n" +
			"turns a flight's or query's results into text — a standup post, a triage\n" +
			"digest, a canned response. Attach one with `formatter: <name>` on a query or\n" +
			"flight, or ad-hoc with `munin fly --formatter <name>`.\n\n" +
			"A formatter replaces the normal output, so it pipes cleanly. The active role\n" +
			"determines which formatters are visible.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFormatterNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return listFormatters(cmd)
			}
			return showFormatter(cmd, args[0])
		},
	}
	c.AddCommand(newFormatterListCmd(), newFormatterShowCmd(), newFormatterRenderCmd())
	return c
}

func newFormatterListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved formatter names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listFormatters(cmd)
		},
	}
}

func newFormatterShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <name>",
		Short:             "Show a saved formatter's definition",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeFormatterNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showFormatter(cmd, args[0])
		},
	}
}

func newFormatterRenderCmd() *cobra.Command {
	var ff formatterFlags
	var stdin bool
	var label string
	c := &cobra.Command{
		Use:   "render <name> [flight]",
		Short: "Run a flight and render its results through a formatter",
		Long: "Render a formatter against results. With a flight name, that flight runs\n" +
			"first; with none, the active role's first flight (or \"default\") runs.\n\n" +
			"With --stdin, no queries run: a JSON section array (the `-o json` wire\n" +
			"format) is read from stdin instead, so `munin fly -o json > f.json` then\n" +
			"`munin formatter render standup --stdin < f.json` gives a zero-latency\n" +
			"edit loop while writing a template.",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeFormatterNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			fd, err := lookupFormatter(args[0])
			if err != nil {
				return err
			}
			if err := requireFormatterOutput(cmd, args[0]); err != nil {
				return err
			}
			o := runOpts{formatter: fd, active: true, copyOut: ff.copyOut, out: ff.out}
			if stdin {
				return renderFormatterStdin(cmd, o, label)
			}
			name := defaultFlightName()
			if len(args) == 2 {
				name = args[1]
			}
			return runFlightNamed(cmd, name, o)
		},
	}
	ff.bindSinks(c)
	c.Flags().BoolVar(&stdin, "stdin", false, "read a JSON section array from stdin instead of running queries")
	c.Flags().StringVar(&label, "label", "stdin", "query label to attribute the stdin sections to")
	return c
}

func renderFormatterStdin(cmd *cobra.Command, o runOpts, label string) error {
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return errs.Wrap(errs.KindUsage, err, "reading stdin")
	}
	var sections []signals.Section
	if err := json.Unmarshal(data, &sections); err != nil {
		return errs.Wrap(errs.KindUsage, err, "parsing stdin as a JSON section array").
			WithHint("pipe the output of `munin fly -o json`")
	}
	o.kind = "query"
	groups := []flightGroup{{Query: label, Title: label, Sections: sections}}
	return deliverGroups(cmd.OutOrStdout(), cmd.ErrOrStderr(), o, label, groups)
}

func listFormatters(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	names := visibleFormatterNames()
	if len(names) == 0 {
		fmt.Fprintln(out, "no formatters visible (check --role, or add a YAML file with a `template:` under ~/.munin/formatters)")
		return nil
	}
	for _, n := range names {
		fd := shared.Directives.Formatters[n]
		lines := len(strings.Split(strings.TrimRight(fd.Template, "\n"), "\n"))
		fmt.Fprintf(out, "%-24s %d line(s)  %s\n", n, lines, theme.Cur().Dim.Render(fd.Title))
	}
	return nil
}

func showFormatter(cmd *cobra.Command, name string) error {
	fd, ok := shared.Directives.Formatters[name]
	if !ok {
		return errs.Newf(errs.KindUsage, "no formatter named %q%s", name, availableFormatterSuffix()).
			WithHint("run `munin formatter` to list saved formatters")
	}
	data, err := yaml.Marshal(fd)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), string(data))
	return nil
}
