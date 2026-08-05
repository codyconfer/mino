package onboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/gitauth"
)

func restore(t *testing.T) {
	origGH, origGit, origGPG := runGH, runGit, runGPG
	origKeygen, origRead := runSSHKeygen, readFile
	origDomain := RequiredEmailDomain
	t.Cleanup(func() {
		runGH, runGit, runGPG = origGH, origGit, origGPG
		runSSHKeygen, readFile = origKeygen, origRead
		RequiredEmailDomain = origDomain
	})
}

// fakeProvider stands in for any git provider. Driving the onboarding checks through
// it rather than through GitHub JSON is what proves the seam is real: nothing below
// knows which forge is on the other side.
type fakeProvider struct {
	label    string
	host     string
	status   gitauth.AuthStatus
	gpg      gitauth.KeyCheck
	ssh      gitauth.KeyCheck
	email    bool
	emailErr error
	calls    []string
}

func (f *fakeProvider) Name() string  { return "fake" }
func (f *fakeProvider) Label() string { return f.label }
func (f *fakeProvider) Host() string  { return f.host }

func (f *fakeProvider) Resolve() (gitauth.Identity, error) { return fakeIdentity{}, nil }

func (f *fakeProvider) Status(context.Context, gitauth.Identity) gitauth.AuthStatus {
	return f.status
}

func (f *fakeProvider) Account(context.Context, gitauth.Identity) (gitauth.Account, error) {
	return gitauth.Account{Login: "fakeuser"}, nil
}

func (f *fakeProvider) RateLimit(context.Context, gitauth.Identity) (gitauth.RateLimit, error) {
	return gitauth.RateLimit{Limit: 5000, Remaining: 4999}, nil
}

func (f *fakeProvider) SigningKeyRegistered(_ context.Context, _ gitauth.Identity, kind gitauth.SigningKeyKind, _ string) gitauth.KeyCheck {
	f.calls = append(f.calls, string(kind))
	if kind == gitauth.SigningSSH {
		return f.ssh
	}
	return f.gpg
}

func (f *fakeProvider) EmailVerified(context.Context, gitauth.Identity, string) (bool, error) {
	return f.email, f.emailErr
}

func (f *fakeProvider) UploadKeyFix(kind gitauth.SigningKeyKind, key string) []string {
	return []string{"upload the " + string(kind) + " key to " + f.host}
}

func (f *fakeProvider) Findings(context.Context, gitauth.Identity) []gitauth.Finding { return nil }

type fakeIdentity struct{ service bool }

func (fakeIdentity) Token(context.Context) (string, error) { return "tok", nil }
func (fakeIdentity) Origin() string                        { return "the fake credential" }
func (fakeIdentity) Authenticated() bool                   { return true }
func (i fakeIdentity) ServiceIdentity() bool               { return i.service }
func (fakeIdentity) Trace() string                         { return "fake: trace" }
func (fakeIdentity) Invalidate()                           {}

var prov *fakeProvider

func allGood() {
	prov = &fakeProvider{
		label:  "FakeHub",
		host:   "fakehub.test",
		status: gitauth.AuthStatus{OK: true, Detail: "fake credential is live"},
		gpg:    gitauth.KeyCheck{Registered: true},
		ssh:    gitauth.KeyCheck{Registered: true},
		email:  true,
	}
	runGH = func(context.Context, ...string) ([]byte, error) { return nil, nil }
	runGit = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "user.signingkey" {
			return []byte("ABCDEF0123456789\n"), nil
		}
		return []byte("true\n"), nil
	}
	runGPG = func(context.Context, ...string) ([]byte, error) { return nil, nil }
}

func check(ctx context.Context) Status { return Check(ctx, prov, fakeIdentity{}) }

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
	st := check(context.Background())
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
	st := check(context.Background())
	if st.Ready() {
		t.Fatal("expected not ready")
	}
	if stepByID(st, StepGitSigningKey).OK {
		t.Error("signing key step should fail")
	}
	if stepByID(st, StepGPGLocal).OK || stepByID(st, StepGPGRemote).OK {
		t.Error("downstream gpg steps should be blocked")
	}
	if len(stepByID(st, StepGitSigningKey).Fix) == 0 {
		t.Error("expected fix hints for missing signing key")
	}
}

func TestCheckSurfacesTheProviderPermissionFix(t *testing.T) {
	restore(t)
	allGood()
	prov.gpg = gitauth.KeyCheck{
		Err: errors.New("not found (HTTP 404)"),
		Fix: []string{"grant the key-read permission on fakehub.test"},
	}
	st := check(context.Background())
	step := stepByID(st, StepGPGRemote)
	if step.OK {
		t.Fatal("expected the remote GPG step to fail when the key list could not be read")
	}
	if len(step.Fix) != 1 || step.Fix[0] != "grant the key-read permission on fakehub.test" {
		t.Fatalf("fix = %+v; onboarding must pass the provider's own remedy through verbatim, because "+
			"only the provider knows how its permissions are granted", step.Fix)
	}
	if !strings.Contains(step.Title, "FakeHub") {
		t.Errorf("title = %q; it must name the configured provider, not a hard-coded forge", step.Title)
	}
}

func TestCheckTokenAuthWithoutGH(t *testing.T) {
	restore(t)
	allGood()
	st := check(context.Background())
	if !stepByID(st, StepProviderAuth).OK {
		t.Fatal("expected github-auth ok via cached token")
	}
}

func TestCheckKeyNotOnGitHub(t *testing.T) {
	restore(t)
	allGood()
	prov.gpg = gitauth.KeyCheck{Registered: false}
	st := check(context.Background())
	if stepByID(st, StepGPGRemote).OK {
		t.Fatal("expected gpg-github to fail when key absent")
	}
}

func TestCheckEmailDomainRequiredPass(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = "example.com"
	prov.gpg = gitauth.KeyCheck{Registered: true, Identities: []string{"dev@example.com"}}
	st := check(context.Background())
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
	prov.gpg = gitauth.KeyCheck{Registered: true, Identities: []string{"dev@other.test"}}
	st := check(context.Background())
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
	prov.gpg = gitauth.KeyCheck{Registered: true, Identities: nil}
	st := check(context.Background())
	if stepByID(st, StepEmailDomain).OK {
		t.Error("unverified matching email must not satisfy the domain step")
	}
}

func TestNoDomainStepWhenUnset(t *testing.T) {
	restore(t)
	allGood()
	RequiredEmailDomain = ""
	st := check(context.Background())
	if stepByID(st, StepEmailDomain).Step == StepEmailDomain {
		t.Error("email-domain step should be absent when no domain is required")
	}
}

const testSSHKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITESTKEY"

func sshGood() {
	allGood()
	runGH = func(context.Context, ...string) ([]byte, error) { return nil, nil }
	runGit = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 {
			switch args[2] {
			case "gpg.format":
				return []byte("ssh\n"), nil
			case "user.signingkey":
				return []byte("key::" + testSSHKey + " comment\n"), nil
			case "user.email":
				return []byte("dev@example.com\n"), nil
			}
		}
		return []byte("true\n"), nil
	}
	prov.ssh = gitauth.KeyCheck{Registered: true}
	prov.email = true
}

func TestCheckSSHAllPass(t *testing.T) {
	restore(t)
	sshGood()
	st := check(context.Background())
	if !st.Ready() {
		t.Fatalf("expected ready; got %+v", st.Results)
	}
	if !stepByID(st, StepSSHLocal).OK || !stepByID(st, StepSSHRemote).OK {
		t.Fatalf("expected ssh steps to pass; got %+v", st.Results)
	}
	if stepByID(st, StepGPGLocal).Step == StepGPGLocal {
		t.Error("gpg steps should be absent in ssh mode")
	}
}

func TestCheckSSHKeyNotRegistered(t *testing.T) {
	restore(t)
	sshGood()
	prov.ssh = gitauth.KeyCheck{Registered: false}
	st := check(context.Background())
	if stepByID(st, StepSSHRemote).OK {
		t.Fatal("expected ssh-github to fail when key not registered")
	}
}

func TestCheckSSHResolvesFromPrivateKeyFile(t *testing.T) {
	restore(t)
	sshGood()
	runGit = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 {
			switch args[2] {
			case "gpg.format":
				return []byte("ssh\n"), nil
			case "user.signingkey":
				return []byte("/home/dev/.ssh/id_ed25519\n"), nil
			case "user.email":
				return []byte("dev@example.com\n"), nil
			}
		}
		return []byte("true\n"), nil
	}
	readFile = func(string) ([]byte, error) { return []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n..."), nil }
	runSSHKeygen = func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte(testSSHKey + " dev@host\n"), nil
	}
	st := check(context.Background())
	if !stepByID(st, StepSSHLocal).OK {
		t.Fatalf("expected ssh-local to resolve via ssh-keygen; got %+v", stepByID(st, StepSSHLocal))
	}
	if !stepByID(st, StepSSHRemote).OK {
		t.Fatalf("expected ssh-github to pass; got %+v", stepByID(st, StepSSHRemote))
	}
}

func TestCheckSSHEmailDomainUnverifiedRejected(t *testing.T) {
	restore(t)
	sshGood()
	RequiredEmailDomain = "example.com"
	prov.email = false
	st := check(context.Background())
	if stepByID(st, StepEmailDomain).OK {
		t.Error("unverified github email must not satisfy the domain step")
	}
}

func TestCheckSSHEmailDomainMismatch(t *testing.T) {
	restore(t)
	sshGood()
	RequiredEmailDomain = "example.com"
	runGit = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 {
			switch args[2] {
			case "gpg.format":
				return []byte("ssh\n"), nil
			case "user.signingkey":
				return []byte("key::" + testSSHKey + "\n"), nil
			case "user.email":
				return []byte("dev@other.test\n"), nil
			}
		}
		return []byte("true\n"), nil
	}
	st := check(context.Background())
	if stepByID(st, StepEmailDomain).OK {
		t.Error("email-domain should fail when user.email is outside the domain")
	}
}

func serviceIdentity(t *testing.T) gitauth.Identity {
	t.Helper()
	return fakeIdentity{service: true}
}

func withServiceAuthBuild(t *testing.T, on bool) {
	t.Helper()
	orig := ServiceAuth
	t.Cleanup(func() { ServiceAuth = orig })
	if on {
		ServiceAuth = "true"
		return
	}
	ServiceAuth = ""
}

func TestServiceAuthSatisfiesTheGateOnlyInAServiceBuild(t *testing.T) {
	restore(t)
	allGood()
	withServiceAuthBuild(t, true)
	// A service build must never consult the human signing seams: there is no human.
	runGit = func(context.Context, ...string) ([]byte, error) {
		t.Error("runGit was called for a service identity; the signing checks are not merely failing " +
			"for a machine, they are inapplicable")
		return nil, errors.New("unreachable")
	}
	runGPG = func(context.Context, ...string) ([]byte, error) {
		t.Error("runGPG was called for a service identity")
		return nil, errors.New("unreachable")
	}
	runSSHKeygen = func(context.Context, ...string) ([]byte, error) {
		t.Error("runSSHKeygen was called for a service identity")
		return nil, errors.New("unreachable")
	}

	st := Check(context.Background(), prov, serviceIdentity(t))
	if !st.Ready() {
		t.Fatalf("a service build is not ready with service auth configured: %+v", st.Results)
	}
	if stepByID(st, StepServiceID).Step != StepServiceID {
		t.Error("no service-identity step in the results; the skip has to be visible in `mino onboard` " +
			"rather than silent")
	}
	if stepByID(st, StepGitSigningKey).Step == StepGitSigningKey {
		t.Error("a signing-key step was reported for a service identity")
	}
}

func TestServiceCredentialsDoNotBypassSigningInAStockBuild(t *testing.T) {
	restore(t)
	allGood()
	withServiceAuthBuild(t, false)
	runGit = func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}

	st := Check(context.Background(), prov, serviceIdentity(t))
	if st.Ready() {
		t.Fatal("a stock build accepted a service credential in place of a signing key. The bypass is " +
			"gated on a compile-time flag precisely so that nothing reachable at runtime — a config " +
			"value, a MINO_* env var, or an exported token — can opt a machine out of commit-signing " +
			"verification. If a config field could do it, anyone could.")
	}
	if stepByID(st, StepServiceID).Step == StepServiceID {
		t.Error("a stock build reported the service-identity step")
	}
}

func TestServiceHintNamesTheMissingBuildFlag(t *testing.T) {
	restore(t)
	withServiceAuthBuild(t, false)
	sel := serviceIdentity(t)

	h := ServiceHint(sel)
	if h == "" {
		t.Fatal("no hint for a service identity in a stock build; the operator configured the credential " +
			"correctly and still sees an onboarding nag, with nothing to explain why")
	}
	if !strings.Contains(h, "SERVICE_AUTH") {
		t.Errorf("hint does not name the build flag: %q", h)
	}
}

func TestServiceHintIsSilentInAServiceBuildAndForHumans(t *testing.T) {
	restore(t)
	withServiceAuthBuild(t, true)
	if h := ServiceHint(serviceIdentity(t)); h != "" {
		t.Errorf("hint = %q in a service build, want none", h)
	}
	withServiceAuthBuild(t, false)
	if h := ServiceHint(fakeIdentity{}); h != "" {
		t.Errorf("hint = %q for a human token, want none; $GITHUB_TOKEN is deliberately in the human "+
			"tier so nobody opts out of signing by exporting one", h)
	}
}
