package auth

import (
	"encoding/json"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

func gpgKeyLookup(raw []byte, signingKey string) (bool, []string) {
	var keys []struct {
		KeyID  string `json:"key_id"`
		Emails []struct {
			Email    string `json:"email"`
			Verified bool   `json:"verified"`
		} `json:"emails"`
		Subkeys []struct {
			KeyID string `json:"key_id"`
		} `json:"subkeys"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false, nil
	}
	want := normalizeGPGKeyID(signingKey)
	if want == "" {
		return false, nil
	}
	found := false
	var ids []string
	for _, k := range keys {
		matched := gpgKeyIDMatch(k.KeyID, want)
		for _, sk := range k.Subkeys {
			if gpgKeyIDMatch(sk.KeyID, want) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		found = true
		for _, e := range k.Emails {
			if e.Verified && e.Email != "" {
				ids = append(ids, e.Email)
			}
		}
	}
	return found, ids
}

func normalizeGPGKeyID(s string) string {
	t := strings.ToUpper(strings.TrimSpace(s))
	t = strings.TrimPrefix(t, "0X")
	if i := strings.LastIndex(t, "!"); i == len(t)-1 && i >= 0 {
		t = t[:i]
	}
	return t
}

func gpgKeyIDMatch(have, want string) bool {
	h := normalizeGPGKeyID(have)
	if h == "" || want == "" {
		return false
	}
	return strings.HasSuffix(h, want) || strings.HasSuffix(want, h)
}

func sshKeyRegistered(raw []byte, pubKey string) bool {
	var keys []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	want := normalizeSSHPubKey(pubKey)
	if want == "" {
		return false
	}
	for _, k := range keys {
		if normalizeSSHPubKey(k.Key) == want {
			return true
		}
	}
	return false
}

func normalizeSSHPubKey(s string) string {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return ""
	}
	return f[0] + " " + f[1]
}

func emailVerified(forge string, raw []byte, email string) (bool, error) {
	var list []struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return false, errs.Wrapf(errs.KindSignal, err, "%s: decoding emails", forge)
	}
	for _, e := range list {
		if e.Verified && strings.EqualFold(e.Email, email) {
			return true, nil
		}
	}
	return false, nil
}
