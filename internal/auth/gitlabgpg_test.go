package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/gitauth"
)

const gpgColonsFixture = `pub:-:255:22:ABCDEF0123456789:1700000000:::-:::scESC:
fpr:::::::::AAAABBBBCCCCDDDDEEEEFFFFABCDEF0123456789:
uid:-::::1700000000::HASH::Cody Confer <cody@example.com>::::::::::0:
uid:-::::1700000000::HASH2::Cody Confer <other@example.com>::::::::::0:
sub:-:255:18:1111222233334444:1700000000::::::e:
`

const gitlabGPGKeysFixture = `[
  {"id": 1, "key": "-----BEGIN PGP PUBLIC KEY BLOCK-----\nmDMEY\n-----END PGP PUBLIC KEY BLOCK-----", "created_at": "2026-01-01T00:00:00Z"}
]`

func stubGPG(t *testing.T, out string, err error) {
	t.Helper()
	prev := gpgRun
	gpgRun = func(context.Context, ...string) ([]byte, error) { return []byte(out), err }
	t.Cleanup(func() { gpgRun = prev })
}

func TestParseGPGColonsReadsPubSubAndUIDs(t *testing.T) {
	k := parseGPGColons(gpgColonsFixture)

	if len(k.KeyIDs) != 2 {
		t.Fatalf("key ids = %v, want the pub and the sub", k.KeyIDs)
	}
	if k.KeyIDs[0] != "ABCDEF0123456789" || k.KeyIDs[1] != "1111222233334444" {
		t.Errorf("key ids = %v", k.KeyIDs)
	}
	if len(k.Emails) != 2 || k.Emails[0] != "cody@example.com" || k.Emails[1] != "other@example.com" {
		t.Errorf("emails = %v, want both uids", k.Emails)
	}
}

func TestParseGPGColonsIgnoresJunk(t *testing.T) {
	k := parseGPGColons("tru::1:1700000000:0:3:1:5\nnot a colon line\n\npub:::\n")
	if len(k.KeyIDs) != 0 || len(k.Emails) != 0 {
		t.Errorf("parsed %+v from junk, want nothing", k)
	}
}

func TestArmoredKeyMatchesSubkeysAndShortIDs(t *testing.T) {
	k := parseGPGColons(gpgColonsFixture)

	cases := []struct {
		key  string
		want bool
	}{
		{"ABCDEF0123456789", true},
		{"0xABCDEF0123456789", true},
		{"ABCDEF0123456789!", true},
		{"abcdef0123456789", true},
		{"0123456789", true},
		{"1111222233334444", true},
		{"9999888877776666", false},
		{"", false},
	}
	for _, c := range cases {
		if got := armoredKeyMatches(k, c.key); got != c.want {
			t.Errorf("armoredKeyMatches(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestGitLabGPGIdentitiesAreIntersectedWithConfirmedEmails(t *testing.T) {
	stubGPG(t, gpgColonsFixture, nil)
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user":          jsonRoute(gitlabUserFixture),
		"user/emails":   jsonRoute(gitlabEmailsFixture),
		"user/gpg_keys": jsonRoute(gitlabGPGKeysFixture),
	})

	got := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningGPG, "ABCDEF0123456789")
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if !got.Registered {
		t.Fatal("the key is registered on the account but was not matched; GitLab returns only the " +
			"armored block, so it has to be parsed locally")
	}
	if len(got.Identities) != 1 || got.Identities[0] != "cody@example.com" {
		t.Errorf("identities = %v, want only the confirmed address; other@example.com is a uid on the "+
			"key but not an address on the account, and returning it would let an unverified address "+
			"satisfy an email-domain requirement", got.Identities)
	}
}

func TestGitLabGPGReportsNoMatchForAnUnknownKey(t *testing.T) {
	stubGPG(t, gpgColonsFixture, nil)
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user":          jsonRoute(gitlabUserFixture),
		"user/emails":   jsonRoute(gitlabEmailsFixture),
		"user/gpg_keys": jsonRoute(gitlabGPGKeysFixture),
	})

	got := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningGPG, "9999888877776666")
	if got.Registered {
		t.Error("an unrelated key id matched")
	}
	if got.Err != nil {
		t.Errorf("a clean miss produced an error: %v", got.Err)
	}
}

func TestGitLabGPGCheckReportsAFixWhenGPGIsMissing(t *testing.T) {
	stubGPG(t, "", errs.New(errs.KindConfig, "gpg is not installed or not on PATH"))
	p, id := gitlabAPI(t, map[string]http.HandlerFunc{
		"user/gpg_keys": jsonRoute(gitlabGPGKeysFixture),
	})

	got := p.SigningKeyRegistered(context.Background(), id, gitauth.SigningGPG, "ABCDEF0123456789")
	if got.Registered {
		t.Fatal("reported a registered key without being able to read it")
	}
	if got.Err == nil {
		t.Fatal("a missing gpg was reported as a clean 'not registered', which sends the user off to " +
			"re-upload a key they already have")
	}
	if len(got.Fix) == 0 || !strings.Contains(strings.Join(got.Fix, " "), "GnuPG") {
		t.Errorf("Fix = %v, want the actionable one", got.Fix)
	}
}

func TestUIDEmailHandlesBareAndBracketedForms(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Cody Confer <cody@example.com>", "cody@example.com"},
		{"cody@example.com", "cody@example.com"},
		{`Cody \x3a Confer <cody@example.com>`, "cody@example.com"},
		{"Cody Confer", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := uidEmail(c.in); got != c.want {
			t.Errorf("uidEmail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
