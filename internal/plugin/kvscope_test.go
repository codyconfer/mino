package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/kv"
)

type recordingKV struct{ namespaces []string }

func (r *recordingKV) Get(_ context.Context, namespace, _ string) (kv.Entry, bool, error) {
	r.namespaces = append(r.namespaces, namespace)
	return kv.Entry{}, false, nil
}

func (r *recordingKV) Put(_ context.Context, namespace, _, _ string, _ time.Time) error {
	r.namespaces = append(r.namespaces, namespace)
	return nil
}

func (r *recordingKV) Delete(_ context.Context, namespace, _ string) error {
	r.namespaces = append(r.namespaces, namespace)
	return nil
}

func (r *recordingKV) List(_ context.Context, namespace string) (map[string]kv.Entry, error) {
	r.namespaces = append(r.namespaces, namespace)
	return nil, nil
}

func TestScopeKVPrefixesEveryNamespace(t *testing.T) {
	ctx := context.Background()
	rec := &recordingKV{}
	scoped := ScopeKV(rec, "acme.tool")

	if _, _, err := scoped.Get(ctx, "cursor", "k"); err != nil {
		t.Fatal(err)
	}
	if err := scoped.Put(ctx, "cursor", "k", "v", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := scoped.Delete(ctx, "ntr", "reminders:default"); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.List(ctx, ""); err != nil {
		t.Fatal(err)
	}

	want := []string{"acme.tool/cursor", "acme.tool/cursor", "acme.tool/ntr", "acme.tool/"}
	if len(rec.namespaces) != len(want) {
		t.Fatalf("namespaces = %v, want %v", rec.namespaces, want)
	}
	for i := range want {
		if rec.namespaces[i] != want[i] {
			t.Fatalf("namespaces = %v, want %v", rec.namespaces, want)
		}
	}
}

func TestScopeKVOwnersCannotAddressEachOther(t *testing.T) {
	a := KVNamespacePrefix("acme.a")
	b := KVNamespacePrefix("acme.b")
	if a == b {
		t.Fatalf("two owners share a prefix: %q", a)
	}
	// A contribution id carries "/" separators; those must not survive into a
	// prefix or a plugin could spell out another owner's namespace.
	if got := KVNamespacePrefix("acme.a/context/tool"); got != "acme.a_context_tool/" {
		t.Fatalf("KVNamespacePrefix = %q", got)
	}
	if got := KVNamespacePrefix(""); got != "unattributed/" {
		t.Fatalf("KVNamespacePrefix(\"\") = %q", got)
	}
}

func TestScopeKVNilStaysNil(t *testing.T) {
	if got := ScopeKV(nil, "acme.tool"); got != nil {
		t.Fatalf("ScopeKV(nil) = %v, want nil", got)
	}
}
