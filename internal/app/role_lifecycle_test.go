package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/role"
)

func hookHome(t *testing.T, cfgBody string) (home, enter, exit string) {
	t.Helper()
	home = t.TempDir()
	markers := t.TempDir()
	enter = filepath.Join(markers, "enter")
	exit = filepath.Join(markers, "exit")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(cfgBody), 0o600); err != nil {
		t.Fatal(err)
	}
	rd := "type: role\nname: hooky\nflights: []\nqueries: []\nhooks:\n  enter:\n    bash: touch " + enter +
		"\n  exit:\n    bash: touch " + exit + "\n"
	if err := os.WriteFile(filepath.Join(home, "hooky.yaml"), []byte(rd), 0o600); err != nil {
		t.Fatal(err)
	}
	other := "type: role\nname: quiet\nflights: []\nqueries: []\n"
	if err := os.WriteFile(filepath.Join(home, "quiet.yaml"), []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, enter, exit
}

func loadHome(t *testing.T, opts Options) *App {
	t.Helper()
	opts.Reconcile = config.ReconcileApply
	a, err := Load(opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(a.Shutdown)
	return a
}

func requireBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("role hooks in this test use bash")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestSessionRoleFlagRunsNoHooksAndWritesNoMarker(t *testing.T) {
	requireBash(t)
	home, enter, exit := hookHome(t, "output: terminal\n")

	a := loadHome(t, Options{Home: home, Role: "hooky"})

	if got := a.Role(); got != "hooky" {
		t.Errorf("Role() = %q, want hooky scoped to the session", got)
	}
	if fileExists(enter) {
		t.Error("--role must not run the enter hook")
	}
	if fileExists(exit) {
		t.Error("--role must not run the exit hook")
	}
	if got := role.LoadActive(home); got != "" {
		t.Errorf("hook marker = %q, want unwritten by --role", got)
	}
}

func TestSessionRoleEnvRunsNoHooksAndWritesNoMarker(t *testing.T) {
	requireBash(t)
	home, enter, _ := hookHome(t, "output: terminal\n")
	t.Setenv("MUNIN_ROLE", "hooky")

	a := loadHome(t, Options{Home: home})

	if got := a.Role(); got != "hooky" {
		t.Errorf("Role() = %q, want hooky from MUNIN_ROLE", got)
	}
	if fileExists(enter) {
		t.Error("MUNIN_ROLE must not run the enter hook")
	}
	if got := role.LoadActive(home); got != "" {
		t.Errorf("hook marker = %q, want unwritten by MUNIN_ROLE", got)
	}
}

func TestRepeatedSessionRoleInvocationsRunNoHooks(t *testing.T) {
	requireBash(t)
	home, enter, exit := hookHome(t, "output: terminal\n")

	for i := 0; i < 4; i++ {
		opts := Options{Home: home}
		if i%2 == 0 {
			opts.Role = "hooky"
		}
		a := loadHome(t, opts)
		a.Shutdown()
	}

	if fileExists(enter) || fileExists(exit) {
		t.Error("alternating --role invocations must not run role hooks")
	}
	if got := role.LoadActive(home); got != "" {
		t.Errorf("hook marker = %q, want unwritten", got)
	}
}

func TestConfigRoleRunsHooksOnceAcrossInvocations(t *testing.T) {
	requireBash(t)
	home, enter, exit := hookHome(t, "role: hooky\n")

	loadHome(t, Options{Home: home}).Shutdown()
	if !fileExists(enter) {
		t.Fatal("role: in config must run the enter hook")
	}
	if got := role.LoadActive(home); got != "hooky" {
		t.Fatalf("hook marker = %q, want hooky", got)
	}

	if err := os.Remove(enter); err != nil {
		t.Fatal(err)
	}
	loadHome(t, Options{Home: home}).Shutdown()
	if fileExists(enter) {
		t.Error("an unchanged role: must not re-run the enter hook")
	}

	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("role: quiet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loadHome(t, Options{Home: home}).Shutdown()
	if !fileExists(exit) {
		t.Error("changing role: must run the previous role's exit hook")
	}
	if got := role.LoadActive(home); got != "quiet" {
		t.Errorf("hook marker = %q, want quiet", got)
	}
}

func TestSessionRoleDoesNotDisturbPersistedRole(t *testing.T) {
	requireBash(t)
	home, enter, exit := hookHome(t, "role: hooky\n")

	loadHome(t, Options{Home: home}).Shutdown()
	if err := os.Remove(enter); err != nil {
		t.Fatal(err)
	}

	loadHome(t, Options{Home: home, Role: "quiet"}).Shutdown()

	if fileExists(exit) {
		t.Error("--role must not run the persisted role's exit hook")
	}
	if fileExists(enter) {
		t.Error("--role must not re-run the enter hook")
	}
	if got := role.LoadActive(home); got != "hooky" {
		t.Errorf("hook marker = %q, want the persisted role hooky", got)
	}
}

func TestActivateRolePersistsRoleAndRunsHooks(t *testing.T) {
	requireBash(t)
	home, enter, exit := hookHome(t, "# keep me\nrole: hooky\n\noutput: terminal # trailing\n")

	a := loadHome(t, Options{Home: home})
	if !fileExists(enter) {
		t.Fatal("initial load should have run the enter hook")
	}

	if err := a.ActivateRole("quiet"); err != nil {
		t.Fatal(err)
	}
	if !fileExists(exit) {
		t.Error("role use should run the previous role's exit hook")
	}
	if got := role.LoadActive(home); got != "quiet" {
		t.Errorf("hook marker = %q, want quiet", got)
	}
	if got := a.Role(); got != "quiet" {
		t.Errorf("Role() = %q, want quiet", got)
	}

	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "role: quiet") {
		t.Errorf("config.yaml did not persist the role:\n%s", got)
	}
	if !strings.Contains(got, "# keep me") || !strings.Contains(got, "# trailing") {
		t.Errorf("config.yaml lost comments:\n%s", got)
	}
}

func TestActivateRoleRejectsUnknownRole(t *testing.T) {
	home, _, _ := hookHome(t, "output: terminal\n")
	a := loadHome(t, Options{Home: home})

	if err := a.ActivateRole("nope"); err == nil {
		t.Fatal("ActivateRole should reject an undefined role")
	}
	if got := a.Role(); got != "" {
		t.Errorf("Role() = %q, want unchanged", got)
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "nope") {
		t.Errorf("a rejected role must not reach config.yaml:\n%s", raw)
	}
}

func TestActivateRoleClearsSessionRole(t *testing.T) {
	requireBash(t)
	home, enter, _ := hookHome(t, "output: terminal\n")

	a := loadHome(t, Options{Home: home, Role: "hooky"})
	if fileExists(enter) {
		t.Fatal("--role must not run hooks on load")
	}
	if err := a.ActivateRole("hooky"); err != nil {
		t.Fatal(err)
	}
	if !fileExists(enter) {
		t.Error("activating the session role should run its enter hook")
	}
	if got := role.LoadActive(home); got != "hooky" {
		t.Errorf("hook marker = %q, want hooky", got)
	}
	if a.transientRole() {
		t.Error("activation should clear the transient session role")
	}
}

func TestTimeoutFlagIsValidated(t *testing.T) {
	home, _, _ := hookHome(t, "output: terminal\n")

	for _, bad := range []string{"notaduration", "30", "-5s", "0", "0s"} {
		a, err := Load(Options{Home: home, Timeout: bad, Reconcile: config.ReconcileApply})
		if err == nil {
			if a != nil {
				a.Shutdown()
			}
			t.Errorf("--timeout %q was accepted", bad)
			continue
		}
		if !strings.Contains(err.Error(), "--timeout") {
			t.Errorf("--timeout %q error = %v, want it to name the flag", bad, err)
		}
	}

	a := loadHome(t, Options{Home: home, Timeout: "45s"})
	if got := a.SourceTimeout(); got != 45*time.Second {
		t.Errorf("SourceTimeout() = %v, want 45s", got)
	}
}

func TestBeginRoleSettleReturnsHooksWithoutRunningThem(t *testing.T) {
	home := t.TempDir()
	orig := role.Run
	role.Run = func(kind, script string) error {
		t.Fatalf("settle must not run hooks inline: %s %s", kind, script)
		return nil
	}
	t.Cleanup(func() { role.Run = orig })
	if err := role.SaveActive(home, "triage"); err != nil {
		t.Fatal(err)
	}

	a := &App{
		Cfg: &config.Config{Home: home, Role: "triage"},
		Directives: &config.Directives{Roles: map[string]config.RoleDef{
			"triage": {Name: "triage", Hooks: config.RoleHooks{
				Exit: config.RoleShellHooks{Bash: "exit-triage", PowerShell: "exit-triage"},
			}},
			"weekly": {Name: "weekly", Hooks: config.RoleHooks{
				Enter: config.RoleShellHooks{Bash: "enter-weekly", PowerShell: "enter-weekly"},
			}},
		}},
	}

	gen, changed := a.BeginRoleCycle("weekly")
	if !changed {
		t.Fatal("BeginRoleCycle should report a change")
	}
	s, ok := a.BeginRoleSettle(gen)
	if !ok {
		t.Fatal("BeginRoleSettle should accept the current generation")
	}
	if len(s.Steps) != 2 {
		t.Fatalf("steps = %+v, want exit then enter", s.Steps)
	}
	if s.Steps[0].Phase != RoleHookExit || s.Steps[0].Role != "triage" {
		t.Errorf("steps[0] = %+v, want triage exit", s.Steps[0])
	}
	if s.Steps[1].Phase != RoleHookEnter || s.Steps[1].Role != "weekly" {
		t.Errorf("steps[1] = %+v, want weekly enter", s.Steps[1])
	}
	for _, step := range s.Steps {
		if step.Script == "" || step.Kind == "" {
			t.Errorf("step %+v is not runnable", step)
		}
		if step.Label() == "" {
			t.Error("step label should describe the hook")
		}
		cmd, err := step.Command()
		if err != nil {
			t.Fatalf("Command() for %+v: %v", step, err)
		}
		if cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
			t.Error("Command() must leave stdio for the caller to wire")
		}
	}
	if got := role.LoadActive(home); got != "triage" {
		t.Errorf("marker = %q, want unchanged until the settle finishes", got)
	}

	if _, ok := a.BeginRoleSettle(gen); ok {
		t.Error("a settled generation must not settle twice")
	}

	a.FinishRoleSettle(s)
	if got := role.LoadActive(home); got != "weekly" {
		t.Errorf("marker = %q, want weekly after FinishRoleSettle", got)
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("FinishRoleSettle should persist role: %v", err)
	}
	if !strings.Contains(string(raw), "role: weekly") {
		t.Errorf("config.yaml = %q, want role: weekly", raw)
	}
}

func TestRoleAndDirectivesAccessorsAreRaceFree(t *testing.T) {
	home := t.TempDir()
	newDirectives := func(extra string) *config.Directives {
		return &config.Directives{Roles: map[string]config.RoleDef{
			"a":   {Name: "a"},
			"b":   {Name: "b"},
			extra: {Name: extra},
		}}
	}
	a := &App{Cfg: &config.Config{Home: home}, Directives: newDirectives("c")}

	const iters = 5000
	names := []string{"a", "b"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iters; i++ {
			a.BeginRoleCycle(names[i%len(names)])
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iters; i++ {
			a.setDirectives(newDirectives("c"))
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iters; i++ {
			if got := a.Role(); got != "" && got != "a" && got != "b" {
				t.Errorf("torn role read: %q", got)
				return
			}
			if d := a.Dirs(); d == nil || len(d.Roles) != 3 {
				t.Error("torn directives read")
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iters; i++ {
			_, _ = a.RoleDef("a")
			_ = a.Access()
			_ = a.VisibleFlights()
		}
	}()

	close(start)
	wg.Wait()
}
