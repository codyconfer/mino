package plugin_test

import (
	"context"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/plugin"
)

type plainCtx struct{}

func (plainCtx) Params() map[string]string { return nil }

func (plainCtx) Home() string { return "" }

func (plainCtx) Role() string { return "" }

type mapKV map[string]string

func (m mapKV) Get(_ context.Context, namespace, key string) (kv.Entry, bool, error) {
	v, ok := m[namespace+"/"+key]
	return kv.Entry{Value: v}, ok, nil
}

func (m mapKV) Put(_ context.Context, namespace, key, value string, _ time.Time) error {
	m[namespace+"/"+key] = value
	return nil
}

func (m mapKV) Delete(_ context.Context, namespace, key string) error {
	delete(m, namespace+"/"+key)
	return nil
}

func (m mapKV) List(context.Context, string) (map[string]kv.Entry, error) { return nil, nil }

type kvCtx struct {
	plainCtx
	kv plugin.KV
}

func (c kvCtx) KV() plugin.KV { return c.kv }

func TestKVOfFetchesTheContextsHandle(t *testing.T) {
	store := mapKV{}
	got := plugin.KVOf(kvCtx{kv: store})
	if got == nil {
		t.Fatal("KVOf returned nil for a context that exposes KV")
	}
	if err := got.Put(context.Background(), "ns", "k", "v", time.Time{}); err != nil {
		t.Fatalf("Put through KVOf handle: %v", err)
	}
	if e, ok, _ := store.Get(context.Background(), "ns", "k"); !ok || e.Value != "v" {
		t.Fatalf("KVOf handle did not reach the backing store: %v %v", e, ok)
	}
}

func TestKVOfIsNilWithoutAKVMethod(t *testing.T) {
	if got := plugin.KVOf(plainCtx{}); got != nil {
		t.Fatalf("KVOf(plainCtx) = %v, want nil", got)
	}
}
