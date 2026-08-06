package auth

import (
	"context"
	"os"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

var gpgRun = GPG

type armoredKey struct {
	KeyIDs []string
	Emails []string
}

func parseArmoredGPGKey(ctx context.Context, armored string) (armoredKey, error) {
	armored = strings.TrimSpace(armored)
	if armored == "" {
		return armoredKey{}, errs.New(errs.KindConfig, "gitlab: empty gpg key block")
	}
	f, err := os.CreateTemp("", "mino-gitlab-gpg-*.asc")
	if err != nil {
		return armoredKey{}, errs.Wrap(errs.KindConfig, err, "gitlab: staging a gpg key for inspection")
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(armored); err != nil {
		f.Close()
		return armoredKey{}, errs.Wrap(errs.KindConfig, err, "gitlab: staging a gpg key for inspection")
	}
	if err := f.Close(); err != nil {
		return armoredKey{}, errs.Wrap(errs.KindConfig, err, "gitlab: staging a gpg key for inspection")
	}

	out, err := gpgRun(ctx, "--with-colons", "--import-options", "show-only", "--import", f.Name())
	if err != nil {
		return armoredKey{}, err
	}
	return parseGPGColons(string(out)), nil
}

func parseGPGColons(out string) armoredKey {
	var k armoredKey
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, ":")
		if len(f) < 5 {
			continue
		}
		switch f[0] {
		case "pub", "sub":
			if id := normalizeGPGKeyID(f[4]); id != "" {
				k.KeyIDs = append(k.KeyIDs, id)
			}
		case "uid":
			if len(f) < 10 {
				continue
			}
			if email := uidEmail(f[9]); email != "" {
				k.Emails = append(k.Emails, email)
			}
		}
	}
	return k
}

func uidEmail(uid string) string {
	uid = strings.TrimSpace(unescapeGPGColon(uid))
	if i := strings.LastIndex(uid, "<"); i >= 0 {
		if j := strings.Index(uid[i:], ">"); j > 0 {
			return strings.TrimSpace(uid[i+1 : i+j])
		}
	}
	if strings.Contains(uid, "@") {
		return uid
	}
	return ""
}

func unescapeGPGColon(s string) string {
	return strings.ReplaceAll(s, `\x3a`, ":")
}

func armoredKeyMatches(k armoredKey, signingKey string) bool {
	want := normalizeGPGKeyID(signingKey)
	if want == "" {
		return false
	}
	for _, id := range k.KeyIDs {
		if gpgKeyIDMatch(id, want) {
			return true
		}
	}
	return false
}
