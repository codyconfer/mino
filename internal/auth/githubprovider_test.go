package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPGKeyLookupMatchesShortAndLongIDsAndSubkeys(t *testing.T) {
	raw := []byte(`[{"key_id":"ABCDEF0123456789","subkeys":[{"key_id":"1111222233334444"}]}]`)
	for _, c := range []struct {
		key  string
		want bool
	}{
		{"ABCDEF0123456789", true},
		{"ABCDEF0123456789!", true},
		{"0123456789", true},
		{"0xABCDEF0123456789", true},
		{"9BFA47F0ABCDEF0123456789", true},
		{"1111222233334444", true},
		{"1111222233334444!", true},
		{"deadbeefdeadbeef", false},
		{"", false},
	} {
		got, _ := gpgKeyLookup(raw, c.key)
		if got != c.want {
			t.Errorf("gpgKeyLookup(%q) = %v, want %v; git config user.signingkey is written in several "+
				"forms (short id, long id, 0x-prefixed, fingerprint, trailing !) and a subkey is as valid "+
				"as the primary, so a strict compare rejects correctly-configured users", c.key, got, c.want)
		}
	}
}

func TestGPGKeyLookupReturnsOnlyVerifiedIdentities(t *testing.T) {
	raw := []byte(`[{"key_id":"ABCDEF0123456789","emails":[` +
		`{"email":"ok@example.com","verified":true},` +
		`{"email":"nope@example.com","verified":false}],"subkeys":[]}]`)
	found, ids := gpgKeyLookup(raw, "ABCDEF0123456789")
	if !found {
		t.Fatal("the key was not found")
	}
	if len(ids) != 1 || ids[0] != "ok@example.com" {
		t.Errorf("identities = %v, want only the verified one; an unverified address must not satisfy a "+
			"domain requirement, since anyone can add one", ids)
	}
}

func TestSSHKeyRegisteredIgnoresComments(t *testing.T) {
	raw := []byte(`[{"key":"ssh-ed25519 AAAAKEYBODY laptop@home"}]`)
	if !sshKeyRegistered(raw, "ssh-ed25519 AAAAKEYBODY someone-else@work") {
		t.Error("the comment field defeated the match; ssh keys are equal on type+body, and the comment " +
			"differs routinely between the local file and what the forge stored")
	}
	if sshKeyRegistered(raw, "ssh-ed25519 AAAAOTHERBODY") {
		t.Error("a different key body matched")
	}
}

func TestGitHubScopeFixNamesTheScopeAndHost(t *testing.T) {
	for _, tc := range []struct {
		name, apiURL, wantHost string
	}{
		{"dotcom", "", "github.com"},
		{"enterprise", "https://ghe.example.com/api/v3", "ghe.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &githubProvider{spec: GitHubSpec{APIURL: tc.apiURL}}
			fix := p.scopeFix(errors.New("gh api user/gpg_keys: gh: Not Found (HTTP 404)"), "admin:gpg_key")
			want := "gh auth refresh -h " + tc.wantHost + " -s admin:gpg_key"
			if len(fix) != 1 || fix[0] != want {
				t.Errorf("fix = %v, want [%s]; the refresh command has to name the host the user is "+
					"actually authenticated against, or it silently refreshes the wrong account", fix, want)
			}
		})
	}
}

func TestGitHubScopeFixIsSilentForUnrelatedErrors(t *testing.T) {
	p := &githubProvider{}
	if fix := p.scopeFix(errors.New("dial tcp: connection refused"), "admin:gpg_key"); fix != nil {
		t.Errorf("fix = %v for a network error; telling the user to re-grant a scope when the host was "+
			"unreachable sends them down the wrong path", fix)
	}
}

func TestGitHubStatusPinsTheConfiguredHostname(t *testing.T) {
	for _, tc := range []struct {
		name, apiURL string
		want         []string
	}{
		{"enterprise", "https://ghe.example.com/api/v3", []string{"auth", "status", "--hostname", "ghe.example.com"}},
		{"dotcom", "", []string{"auth", "status"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			argsFile := filepath.Join(t.TempDir(), "gh-args")
			fakeGH(t, "#!/bin/sh\necho \"$@\" > "+argsFile+"\n")

			p := &githubProvider{spec: GitHubSpec{APIURL: tc.apiURL, Store: memStore{}}}
			id, err := p.Resolve()
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if st := p.Status(context.Background(), id); !st.OK {
				t.Fatalf("Status = %+v, want OK with a fake gh that succeeds", st)
			}
			got, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("gh was never invoked: %v", err)
			}
			if strings.TrimSpace(string(got)) != strings.Join(tc.want, " ") {
				t.Errorf("gh args = %q, want %q", strings.TrimSpace(string(got)), strings.Join(tc.want, " "))
			}
		})
	}
}
