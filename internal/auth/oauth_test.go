package auth

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestLoopbackAuthCode(t *testing.T) {

	orig := openBrowser
	openBrowser = func(string) error { return nil }
	defer func() { openBrowser = orig }()

	code, redirect, err := loopbackAuthCode(context.Background(), io.Discard, "Test",
		func(redirect, state string) string {

			go http.Get(redirect + "?code=the-code&state=" + state)
			return "https://provider/authorize?state=" + state
		})
	if err != nil {
		t.Fatal(err)
	}
	if code != "the-code" {
		t.Errorf("code = %q, want the-code", code)
	}
	if redirect == "" {
		t.Error("expected a non-empty redirect URI")
	}
}

func TestLoopbackAuthCodeStateMismatch(t *testing.T) {
	orig := openBrowser
	openBrowser = func(string) error { return nil }
	defer func() { openBrowser = orig }()

	_, _, err := loopbackAuthCode(context.Background(), io.Discard, "Test",
		func(redirect, _ string) string {
			go http.Get(redirect + "?code=x&state=WRONG")
			return "https://provider/authorize"
		})
	if err == nil {
		t.Fatal("expected a state-mismatch error")
	}
}
