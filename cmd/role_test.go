package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func roleSubcommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	role := findCmd(Root(), "role")
	if role == nil {
		t.Fatal("missing top-level `role` command")
	}
	sub := findCmd(role, name)
	if sub == nil {
		t.Fatalf("missing `role %s` subcommand", name)
	}
	return sub
}

func TestRoleUseTakesExactlyOneArg(t *testing.T) {
	use := roleSubcommand(t, "use")
	for _, args := range [][]string{{}, {"a", "b"}} {
		if err := use.Args(use, args); err == nil {
			t.Errorf("Args(%v) = nil, want an error", args)
		}
	}
	if err := use.Args(use, []string{"focus"}); err != nil {
		t.Errorf("Args([focus]) = %v", err)
	}
	if use.ValidArgsFunction == nil {
		t.Error("`role use` should complete role names")
	}
}

func TestRoleUseRejectsUnknownRoleLoudly(t *testing.T) {
	withDirectives(t)
	use := roleSubcommand(t, "use")

	err := use.RunE(use, []string{"typo"})
	if err == nil {
		t.Fatal("`role use typo` should fail")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error = %v, want it to name the bad role", err)
	}
	if got := shared.Role(); got != "" {
		t.Errorf("role = %q, want unchanged after a rejected name", got)
	}
}

func TestRoleUseRejectsAnyNameWhenNoRolesAreDefined(t *testing.T) {
	shared = &app.App{Cfg: &config.Config{Home: t.TempDir()}, Directives: &config.Directives{}}
	t.Cleanup(func() { shared = nil })

	use := roleSubcommand(t, "use")
	if err := use.RunE(use, []string{"anything"}); err == nil {
		t.Fatal("`role use` should fail when no roles are defined")
	}
}

func TestRoleUsePersistsTheRole(t *testing.T) {
	withDirectives(t)
	home := shared.Cfg.Home
	use := roleSubcommand(t, "use")
	var out bytes.Buffer
	use.SetOut(&out)

	if err := use.RunE(use, []string{"focus"}); err != nil {
		t.Fatal(err)
	}
	if got := shared.Role(); got != "focus" {
		t.Errorf("role = %q, want focus", got)
	}
	if !strings.Contains(out.String(), "focus") {
		t.Errorf("output = %q, want it to confirm the role", out.String())
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("role use should write config.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "role: focus") {
		t.Errorf("config.yaml = %q, want role: focus", raw)
	}
}

func TestRoleClearRemovesTheRole(t *testing.T) {
	withDirectives(t)
	home := shared.Cfg.Home
	if err := shared.ActivateRole("focus"); err != nil {
		t.Fatal(err)
	}

	clear := roleSubcommand(t, "clear")
	var out bytes.Buffer
	clear.SetOut(&out)
	if err := clear.RunE(clear, nil); err != nil {
		t.Fatal(err)
	}
	if got := shared.Role(); got != "" {
		t.Errorf("role = %q, want cleared", got)
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "role: focus") {
		t.Errorf("config.yaml = %q, want the role cleared", raw)
	}
}

func TestRoleHelpDocumentsTransientFlag(t *testing.T) {
	role := findCmd(Root(), "role")
	if role == nil {
		t.Fatal("missing role command")
	}
	for _, want := range []string{"role use", "--role", "MINO_ROLE", "no hooks"} {
		if !strings.Contains(role.Long, want) {
			t.Errorf("role help does not mention %q:\n%s", want, role.Long)
		}
	}
}
