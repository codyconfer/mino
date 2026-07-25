//go:build !nodaemon

package cmd

import "testing"

func TestDaemonCommandTree(t *testing.T) {
	root := Root()

	serve := findCmd(root, "serve")
	if serve == nil {
		t.Fatal("missing top-level command \"serve\"")
	}
	if serve.Flags().Lookup("tui") != nil || serve.Flags().Lookup("tray") != nil {
		t.Error("serve must not expose --tui or --tray")
	}

	daemon := findCmd(root, "daemon")
	if daemon == nil {
		t.Fatal("missing top-level command \"daemon\"")
	}
	for _, s := range []string{"install", "uninstall", "start", "stop", "restart", "status", "attach"} {
		if findCmd(daemon, s) == nil {
			t.Errorf("daemon missing subcommand %q", s)
		}
	}
	run := findCmd(daemon, "run")
	if run == nil {
		t.Error("daemon missing hidden run entrypoint for the OS service")
	} else if !run.Hidden {
		t.Error("daemon run should be hidden (OS service entrypoint)")
	}
}

func TestDaemonGateMode(t *testing.T) {
	root := Root()
	want := map[string]string{
		"serve":  modeServe,
		"daemon": modeDaemon,
	}
	for name, exp := range want {
		c := findCmd(root, name)
		if c == nil {
			t.Fatalf("no command %q", name)
		}
		if got := gateMode(c); got != exp {
			t.Errorf("gateMode(%s) = %q, want %q", name, got, exp)
		}
	}

	install := findCmd(findCmd(root, "daemon"), "install")
	if got := gateMode(install); got != modeDaemon {
		t.Errorf("gateMode(daemon install) = %q, want %q", got, modeDaemon)
	}
}
