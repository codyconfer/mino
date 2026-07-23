package onboard

import (
	"context"
	"errors"
	"testing"

	"github.com/codyconfer/munin/internal/auth"
)

type fakeStore struct{}

func (fakeStore) Get(string) (auth.Credential, bool, error) { return auth.Credential{}, false, nil }
func (fakeStore) Put(string, auth.Credential) error         { return nil }
func (fakeStore) Delete(string) error                       { return nil }

func restore(t *testing.T) {
	origAvail, origTok := ghAvailable, ghToken
	origGH, origGit, origGPG, origAPI := runGH, runGit, runGPG, ghAPIGet
	origDomain := RequiredEmailDomain
	t.Cleanup(func() {
		ghAvailable, ghToken = origAvail, origTok
		runGH, runGit, runGPG, ghAPIGet = origGH, origGit, origGPG, origAPI
		RequiredEmailDomain = origDomain
	})
}

func allGood() {
	ghAvailable = func() bool { return true }
	runGH = func(context.Context, ...string) ([]byte, error) { return nil, nil }
	ghToken = func(auth.TokenStore) (string, string) { return "", "" }
	runGit = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "user.signingkey" {
			return []byte("ABCDEF0123456789\n"), nil
		}
		return []byte("true\n"), nil
	}
	runGPG = func(context.Context, ...string) ([]byte, error) { return nil, nil }
	ghAPIGet = func(context.Context, auth.TokenStore, string, string) ([]byte, error) {
		return []byte(`[{"key_id":"ABCDEF0123456789","subkeys":[]}]`), nil
	}
}

func stepByID(st Status, id StepID) Result {
	for _, r := range st.Results {
		if r.Step == id {
			return r
		}
	}
	return Result{}
}

func TestCheckAllPass(t *testing.T) {
	restore(t)
	allGood()
	st := Check(context.Background(), fakeStore{}, "")
	if !st.Ready() {
		t.Fatalf("expected ready; got %+v", st.Results)
	}
	if len(st.Results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(st.Results))
	}
}

func TestCheckMissingSigningKeyBlocksDownstream(t *testing.T) {
	restore(t)
	allGood()
	runGit = func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}
	st := Check(context.Background(), fakeStore{}, "")
	if st.Ready() {
		t.Fatal("expected not ready")
	}
	if stepByID(st, StepGitSigningKey).OK {
		t.Error("signing key step should fail")
	}
	if stepByID(st, StepGPGLocal).OK || stepByID(st, StepGPGGitHub).OK {
		t.Error("downstream gpg steps should be blocked")
	}
	if len(stepByID(st, StepGitSigningKey).Fix) == 0 {
		t.Error("expected fix hints for missing signing key")
	}
}

func TestCheckTokenAuthWithoutGH(t *testing.T) {
	restore(t)
	allGood()
	ghAvailable = func() bool { return false }
	ghToken = func(auth.TokenStore) (string, string) { return "tok", "cached OAuth token" }
	st := Check(context.Background(), fakeStore{}, "")
	if !stepByID(st, StepGitHubAuth).OK {
		t.Fatal("expected github-auth ok via cached token")
	}
}

func TestCheckKeyNotOnGitHub(t *testing.T) {
	restore(t)
	allGood()
	ghAPIGet = func(context.Context, auth.TokenStore, string, string) ([]byte, error) {
		return []byte(`[{"key_id":"0000000000000000","subkeys":[]}]`), nil
	}
	st := Check(context.Background(), fakeStore{}, "")
	if stepByID(st, StepGPGGitHub).OK {
		t.Fatal("expected gpg-github to fail when key absent")
	}
}

func TestCheckEmailDomainRequiredPass(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = "example.com"
	ghAPIGet = func(context.Context, auth.TokenStore, string, string) ([]byte, error) {
		return []byte(`[{"key_id":"ABCDEF0123456789","emails":[{"email":"dev@example.com","verified":true}],"subkeys":[]}]`), nil
	}
	st := Check(context.Background(), fakeStore{}, "")
	if !st.Ready() {
		t.Fatalf("expected ready with matching domain; got %+v", st.Results)
	}
	if len(st.Results) != 5 {
		t.Fatalf("expected 5 results with domain step, got %d", len(st.Results))
	}
	if !stepByID(st, StepEmailDomain).OK {
		t.Error("email-domain step should pass")
	}
}

func TestCheckEmailDomainMismatch(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = "example.com"
	ghAPIGet = func(context.Context, auth.TokenStore, string, string) ([]byte, error) {
		return []byte(`[{"key_id":"ABCDEF0123456789","emails":[{"email":"dev@other.test","verified":true}],"subkeys":[]}]`), nil
	}
	st := Check(context.Background(), fakeStore{}, "")
	if st.Ready() {
		t.Fatal("expected not ready when no verified email matches the domain")
	}
	if stepByID(st, StepEmailDomain).OK {
		t.Error("email-domain step should fail on mismatch")
	}
}

func TestCheckEmailDomainUnverifiedRejected(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = "example.com"
	ghAPIGet = func(context.Context, auth.TokenStore, string, string) ([]byte, error) {
		return []byte(`[{"key_id":"ABCDEF0123456789","emails":[{"email":"dev@example.com","verified":false}],"subkeys":[]}]`), nil
	}
	st := Check(context.Background(), fakeStore{}, "")
	if stepByID(st, StepEmailDomain).OK {
		t.Error("unverified matching email must not satisfy the domain step")
	}
}

func TestNoDomainStepWhenUnset(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = ""
	st := Check(context.Background(), fakeStore{}, "")
	if stepByID(st, StepEmailDomain).Step == StepEmailDomain {
		t.Error("email-domain step should be absent when no domain is required")
	}
}

func TestKeyRegistered(t *testing.T) {
	raw := []byte(`[{"key_id":"ABCDEF0123456789","subkeys":[{"key_id":"1111222233334444"}]}]`)
	cases := []struct {
		key  string
		want bool
	}{
		{"ABCDEF0123456789", true},
		{"0123456789", true},
		{"0xABCDEF0123456789", true},
		{"9BFA47F0ABCDEF0123456789", true},
		{"1111222233334444", true},
		{"deadbeefdeadbeef", false},
		{"", false},
	}
	for _, c := range cases {
		if got := keyRegistered(raw, c.key); got != c.want {
			t.Errorf("keyRegistered(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}
