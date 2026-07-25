package views

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
)

func TestAuditDefaultSQLMatchesSchemas(t *testing.T) {
	me := &auditView{}
	for i, db := range auditvDBs {
		me.dbIndex = i
		q := me.defaultSQL()
		if q == "" {
			t.Fatalf("defaultSQL(%q) empty", db)
		}
		if !auditvReadOnly(q) {
			t.Fatalf("defaultSQL(%q) not read-only: %s", db, q)
		}
	}
	if !strings.Contains(auditvDefaultSQL["audit"], "FROM runs") {
		t.Fatalf("audit default = %q", auditvDefaultSQL["audit"])
	}
	if !strings.Contains(auditvDefaultSQL["audit"], "sum(count)") {
		t.Fatalf("audit default should sum item counts: %q", auditvDefaultSQL["audit"])
	}
	if !strings.Contains(auditvDefaultSQL["config"], "FROM store_current") {
		t.Fatalf("config default = %q", auditvDefaultSQL["config"])
	}
	if !strings.Contains(auditvDefaultSQL["tokens"], "FROM kv") {
		t.Fatalf("tokens default = %q", auditvDefaultSQL["tokens"])
	}
}

func TestAuditQueryDefaultAgainstAuditDB(t *testing.T) {
	home := t.TempDir()
	st, err := audit.Open(context.Background(), config.DataPath(home, "audit.duckdb"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	start := time.Now()
	fid := st.StartFlight("morning", "triage")
	st.RecordQuery(fid, "incidents", "triage", start, time.Now(), []signals.Section{{
		Signal: "github",
		Items:  []signals.Item{{Kind: "pr", Title: "one", Timestamp: start}},
	}})
	st.FinishFlight(fid)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home, dbIndex: 0}
	out, err := me.exec(me.defaultSQL())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "morning") && !strings.Contains(out, "incidents") {
		t.Fatalf("result missing run names: %q", out)
	}
	if !strings.Contains(out, "runs") || !strings.Contains(out, "items") {
		t.Fatalf("result missing aggregate columns: %q", out)
	}
}

func TestAuditQueryDefaultAgainstConfigDB(t *testing.T) {
	home := t.TempDir()
	db, err := configdb.Open(context.Background(), config.DataPath(home, "config.duckdb"))
	if err != nil {
		t.Fatalf("configdb.Open: %v", err)
	}
	if err := db.Import(context.Background(), "queries", []byte("name: demo\nsignal: demo\n"), "collection"); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home}
	me.dbIndex = indexOfDB("config")
	out, err := me.exec(me.defaultSQL())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "queries") {
		t.Fatalf("result missing store name: %q", out)
	}
}

func TestAuditQueryDefaultAgainstTokensDB(t *testing.T) {
	home := t.TempDir()
	store, err := kv.Open(context.Background(), config.DataPath(home, "tokens.duckdb"))
	if err != nil {
		t.Fatalf("kv.Open: %v", err)
	}
	if err := store.Put(context.Background(), "tokens", "github", "sealed", time.Time{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	me := &auditView{home: home}
	me.dbIndex = indexOfDB("tokens")
	out, err := me.exec(me.defaultSQL())
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(out, "github") || !strings.Contains(out, "tokens") {
		t.Fatalf("result missing kv row: %q", out)
	}
	if strings.Contains(out, "sealed") {
		t.Fatalf("token value should not be selected: %q", out)
	}
}

func indexOfDB(name string) int {
	for i, n := range auditvDBs {
		if n == name {
			return i
		}
	}
	return 0
}
