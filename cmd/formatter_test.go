package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

func useFormatterTestApp(t *testing.T, output string) {
	t.Helper()
	t.Setenv(envOutput, "")
	orig := shared
	t.Cleanup(func() { shared = orig })
	shared = &app.App{
		Cfg: &config.Config{Home: t.TempDir(), Output: output, Timeout: "5s"},
		Directives: verifyDirectives(t, map[string]string{
			"flights/demo.yaml":       "name: demo\ntype: flight\nqueries: [ghost]\nformatter: standup\n",
			"flights/plain.yaml":      "name: plain\ntype: flight\nqueries: [ghost]\n",
			"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"## Standup {{ .Count }} items\\n\"\n",
		}),
	}
}

func flyTestRoot(t *testing.T) *cobra.Command {
	t.Helper()
	orig := flagOutput
	t.Cleanup(func() { flagOutput = orig })
	root := &cobra.Command{
		Use:           "munin",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if flagOutput != "" {
				shared.Cfg.Output = flagOutput
			}
			return nil
		},
	}
	root.PersistentFlags().StringVarP(&flagOutput, "output", "o", "", "output format: terminal or json")
	root.AddCommand(newFlyCmd())
	return root
}

func runFly(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := flyTestRoot(t)
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"fly"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestExplicitJSONOutputBeatsAConfigFormatter(t *testing.T) {
	useFormatterTestApp(t, "terminal")

	out, _ := runFly(t, "-o", "json", "demo")
	if strings.Contains(out, "Standup") {
		t.Fatalf("-o json rendered the config formatter instead of JSON:\n%s", out)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("-o json did not emit JSON:\n%s", out)
	}
}

func TestConfigFormatterStillAppliesWithoutAnExplicitOutputRequest(t *testing.T) {
	useFormatterTestApp(t, "json")

	out, _ := runFly(t, "demo")
	if !strings.Contains(out, "Standup") {
		t.Fatalf("a config-level formatter should still render when no output was requested for this run:\n%s", out)
	}
}

func TestEnvOutputBeatsAConfigFormatter(t *testing.T) {
	useFormatterTestApp(t, "json")
	t.Setenv(envOutput, "json")

	out, _ := runFly(t, "demo")
	if strings.Contains(out, "Standup") {
		t.Fatalf("MUNIN_OUTPUT=json rendered the config formatter instead of JSON, so "+
			"`MUNIN_OUTPUT=json munin fly | jq` gets a text report:\n%s", out)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("MUNIN_OUTPUT=json did not emit JSON:\n%s", out)
	}
}

func TestEnvOutputWithAnExplicitFormatterIsAUsageError(t *testing.T) {
	useFormatterTestApp(t, "json")
	t.Setenv(envOutput, "json")

	out, err := runFly(t, "--formatter", "standup", "plain")
	if err == nil {
		t.Fatalf("--formatter under MUNIN_OUTPUT=json exited 0:\n%s", out)
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want KindUsage (err: %v)", errs.KindOf(err), err)
	}
	if !strings.Contains(err.Error(), "MUNIN_OUTPUT=json") {
		t.Errorf("err = %v, want it to name MUNIN_OUTPUT as the thing to change", err)
	}
}

func TestUnknownOutputFormatFailsEvenWithAFormatter(t *testing.T) {
	useFormatterTestApp(t, "terminal")

	out, err := runFly(t, "-o", "xml", "demo")
	if err == nil {
		t.Fatalf("-o xml with a formatter exited 0 and ignored the format:\n%s", out)
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("err = %v, want it to name the unknown format", err)
	}
	if strings.Contains(out, "Standup") {
		t.Errorf("-o xml still rendered the formatter:\n%s", out)
	}
}

func TestExplicitFormatterWithIncompatibleOutputIsAUsageError(t *testing.T) {
	useFormatterTestApp(t, "terminal")

	out, err := runFly(t, "--formatter", "standup", "-o", "json", "plain")
	if err == nil {
		t.Fatalf("--formatter with -o json exited 0:\n%s", out)
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want KindUsage (err: %v)", errs.KindOf(err), err)
	}
	if strings.Contains(out, "Standup") {
		t.Errorf("the conflicting run still produced formatter output:\n%s", out)
	}
}
