package views

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/testenv"
)

func storeKit(t *testing.T) *Kit {
	t.Helper()
	testenv.Isolate(t)
	home := t.TempDir()
	mgr, err := config.OpenStore(context.Background(), home)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })

	cfg := config.Defaults()
	cfg.Home = home
	return New(Deps{
		App: &app.App{
			Cfg: cfg,
			Mgr: mgr,
			Directives: &config.Directives{
				Queries: map[string]config.Query{},
				Flights: map[string]config.Flight{},
				Roles:   map[string]config.RoleDef{},
			},
		},
		FetchQuery:         func(string) []signals.Section { return nil },
		FetchFlightAudited: func(string) []signals.Section { return nil },
	})
}

func applyFromAnotherProcess(t *testing.T, kit *Kit) {
	t.Helper()
	home := kit.d.App.Cfg.Home
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("standup-q.yaml", "name: standup-q\ntype: query\nsignal: github\n")
	write("standup.yaml", "name: standup\ntype: flight\nqueries: [standup-q]\n")
	blob, has, err := config.SerializeDirectives(home)
	if err != nil || !has {
		t.Fatalf("serialize directives = has %v, err %v", has, err)
	}
	if err := kit.d.App.Mgr.DB().Import(context.Background(), config.DirectivesDirective, blob, "collection"); err != nil {
		t.Fatalf("import: %v", err)
	}
}

func TestStoreTickReloadsAfterExternalApply(t *testing.T) {
	kit := storeKit(t)
	hook := kit.MsgHook()
	m := vkdeck.New(vkdeck.NewMenu("root", nil))

	if _, handled := hook(m, storeTickMsg{}); !handled {
		t.Fatal("store tick was not handled")
	}
	if _, ok := kit.d.App.Directives.Flights["standup"]; ok {
		t.Fatal("flight present before it was applied")
	}

	applyFromAnotherProcess(t, kit)

	cmd, handled := hook(m, storeTickMsg{})
	if !handled {
		t.Fatal("store tick after an external apply was not handled")
	}
	if cmd == nil {
		t.Fatal("store tick after an external apply produced no command")
	}
	if _, ok := kit.d.App.Directives.Flights["standup"]; !ok {
		t.Fatalf("directives did not pick up the applied flight: %v", kit.d.App.Directives.FlightNames())
	}
}

func TestStoreTickRearmsWhenUnchanged(t *testing.T) {
	kit := storeKit(t)
	hook := kit.MsgHook()
	m := vkdeck.New(vkdeck.NewMenu("root", nil))

	for i := range 2 {
		cmd, handled := hook(m, storeTickMsg{})
		if !handled {
			t.Fatalf("tick %d was not handled", i)
		}
		if cmd == nil {
			t.Fatalf("tick %d did not re-arm the watcher", i)
		}
		if i == 0 {
			if _, ok := cmd().(storeTickMsg); !ok {
				t.Fatalf("tick %d re-armed with the wrong message", i)
			}
		}
	}
}

func TestStoreTickSettlesAfterReload(t *testing.T) {
	kit := storeKit(t)
	hook := kit.MsgHook()
	m := vkdeck.New(vkdeck.NewMenu("root", nil))

	hook(m, storeTickMsg{})
	applyFromAnotherProcess(t, kit)
	hook(m, storeTickMsg{})

	rev, ok := kit.d.App.StoreRevision()
	if !ok {
		t.Fatal("no revision after apply")
	}
	hook(m, storeTickMsg{})
	if got, _ := kit.d.App.StoreRevision(); got != rev {
		t.Fatalf("revision moved without a write: %q -> %q", rev, got)
	}
	if kit.storeRev != rev {
		t.Fatalf("watcher tracking %q, store at %q", kit.storeRev, rev)
	}
}
