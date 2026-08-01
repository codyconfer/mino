package cmd

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/config"
)

func TestMaskSecrets(t *testing.T) {
	in := strings.Join([]string{
		"oauth_client_id: Iv1.abc123",
		"oauth_client_secret: super-secret-value",
		"token_env: SLACK_TOKEN",
		"access_token: ya29.leaky",
		"query: is:open is:pr",
	}, "\n")
	got := redact.Line(in)

	if strings.Contains(got, "super-secret-value") {
		t.Error("client secret was not masked")
	}
	if strings.Contains(got, "ya29.leaky") {
		t.Error("access_token was not masked")
	}

	if !strings.Contains(got, "Iv1.abc123") {
		t.Error("client_id (not a secret) should be preserved")
	}
	if !strings.Contains(got, "SLACK_TOKEN") {
		t.Error("token_env is a var name, not a secret; should be preserved")
	}
	if !strings.Contains(got, "is:open is:pr") {
		t.Error("query value should be preserved")
	}
}

func TestSecretKey(t *testing.T) {
	for _, k := range []string{"oauth_client_secret", "password", "access_token", "refresh_token"} {
		if !redact.IsSecretKey(k) {
			t.Errorf("%q should be treated as secret", k)
		}
	}
	for _, k := range []string{"oauth_client_id", "token_env", "query", "name"} {
		if redact.IsSecretKey(k) {
			t.Errorf("%q should NOT be treated as secret", k)
		}
	}
}

func verifyDirectives(t *testing.T, files map[string]string) *config.Directives {
	t.Helper()
	blob, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	d, err := config.ParseDirectives(blob)
	if err != nil {
		t.Fatalf("ParseDirectives: %v", err)
	}
	return d
}

func TestVerifyAdvertisesTheFormattersTarget(t *testing.T) {
	c := findCmd(Root(), "verify")
	if c == nil {
		t.Fatal("missing top-level `verify` command")
	}
	if !strings.Contains(c.Use, "formatters") {
		t.Errorf("Use = %q, want it to list the formatters target", c.Use)
	}
	if !slices.Contains(c.ValidArgs, config.KindFormatters) {
		t.Errorf("ValidArgs = %v, want %q", c.ValidArgs, config.KindFormatters)
	}
	if !strings.Contains(c.Long, "formatter") {
		t.Errorf("Long = %q, want it to mention formatter integrity", c.Long)
	}
}

func TestVerifyRejectsAnUnknownTarget(t *testing.T) {
	shared = &app.App{
		Cfg:        &config.Config{Home: t.TempDir(), Output: "terminal"},
		Directives: verifyDirectives(t, map[string]string{}),
	}

	var buf bytes.Buffer
	c := newVerifyCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs([]string{"flight"})
	c.SilenceErrors = true
	c.SilenceUsage = true
	if err := c.Execute(); err == nil {
		t.Fatalf("`verify flight` (typo) exited 0 while validating nothing:\n%s", buf.String())
	}
}

func TestVerifyAcceptsEveryAdvertisedTarget(t *testing.T) {
	c := newVerifyCmd()
	targets := verify.Targets()
	for _, want := range []string{"config", "roles", "flights", "queries", "formatters", "plugins", "onboarding", "all"} {
		if !slices.Contains(targets, want) {
			t.Errorf("verify.Targets() = %v, want it to include %q", targets, want)
		}
		if !slices.Contains(c.ValidArgs, want) {
			t.Errorf("ValidArgs = %v, want it to include %q", c.ValidArgs, want)
		}
		if !strings.Contains(c.Use, want) {
			t.Errorf("Use = %q, want it to document the %q target", c.Use, want)
		}
		if err := c.Args(c, []string{want}); err != nil {
			t.Errorf("Args(%q) = %v, want it accepted", want, err)
		}
	}
	if err := c.Args(c, []string{"flight"}); err == nil {
		t.Error("Args accepted the unknown target \"flight\"")
	}
}

func TestVerifyRunsTheFormattersTarget(t *testing.T) {
	shared = &app.App{
		Cfg: &config.Config{Home: t.TempDir(), Output: "terminal"},
		Directives: verifyDirectives(t, map[string]string{
			"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }} items\"\n",
		}),
	}

	var buf bytes.Buffer
	c := newVerifyCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs([]string{config.KindFormatters})
	if err := c.Execute(); err != nil {
		t.Fatalf("verify formatters = %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Formatters") || !strings.Contains(out, "standup") {
		t.Errorf("output missing the formatters section:\n%s", out)
	}
}

func TestVerifyFormattersReportsProblems(t *testing.T) {
	d := verifyDirectives(t, map[string]string{
		"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }} items\"\n",
	})
	d.Formatters["bare"] = config.FormatterDef{Name: "bare", Type: config.TypeFormatter}
	shared = &app.App{Cfg: &config.Config{Home: t.TempDir(), Output: "terminal"}, Directives: d}

	var buf bytes.Buffer
	c := newVerifyCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs([]string{config.KindFormatters})
	c.SilenceErrors = true
	err := c.Execute()
	if err == nil {
		t.Fatalf("want a problem for the template-less formatter\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "1 problem(s) found") {
		t.Errorf("err = %v, want the problem count", err)
	}
}

func TestVerifyFindingsRoutesFormatters(t *testing.T) {
	shared = &app.App{
		Cfg: &config.Config{Home: t.TempDir()},
		Directives: verifyDirectives(t, map[string]string{
			"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }} items\"\n",
		}),
	}

	findings := verifyFindings(config.KindFormatters)
	if len(findings) != 1 || findings[0].Name != "standup" || !findings[0].OK {
		t.Fatalf("verifyFindings(%q) = %+v", config.KindFormatters, findings)
	}
	if got := verifyFindings("nope"); got != nil {
		t.Errorf("verifyFindings on an unknown kind = %+v, want nil", got)
	}
}
