package serve

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBindDeckLifelineNoEnv(t *testing.T) {
	t.Setenv(deckLifelineEnv, "")
	ctx, cancel := BindDeckLifeline(context.Background())
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("context cancelled without lifeline")
	default:
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("manual cancel did not finish")
	}
}

func TestBindDeckLifelineBadFD(t *testing.T) {
	t.Setenv(deckLifelineEnv, "not-a-number")
	ctx, cancel := BindDeckLifeline(context.Background())
	defer cancel()
	if ctx.Err() != nil {
		t.Fatal("expected live context for bad fd env")
	}
	_ = os.Unsetenv(deckLifelineEnv)
}
