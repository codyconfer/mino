package onboard

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/codyconfer/munin/internal/auth"
)

var RequiredEmailDomain string

type StepID string

const (
	StepGitHubAuth    StepID = "github-auth"
	StepGitSigningKey StepID = "git-signingkey"
	StepGPGLocal      StepID = "gpg-local"
	StepGPGGitHub     StepID = "gpg-github"
	StepEmailDomain   StepID = "email-domain"
)

type Result struct {
	Step   StepID
	Title  string
	OK     bool
	Detail string
	Fix    []string
}

type Status struct {
	Results []Result
}

func (s Status) Ready() bool {
	if len(s.Results) == 0 {
		return false
	}
	for _, r := range s.Results {
		if !r.OK {
			return false
		}
	}
	return true
}

var (
	ghAvailable = auth.GHAvailable
	ghToken     = auth.GitHubToken
	runGH       = auth.GH
	runGit      = auth.Git
	runGPG      = auth.GPG
	ghAPIGet    = auth.GHAPIGet
)

func Check(ctx context.Context, tokens auth.TokenStore, apiURL string) Status {
	var st Status

	authOK, authRes := checkGitHubAuth(ctx, tokens)
	st.Results = append(st.Results, authRes)

	signingKey, keyRes := checkSigningKey(ctx)
	st.Results = append(st.Results, keyRes)

	st.Results = append(st.Results, checkGPGLocal(ctx, signingKey))

	var raw []byte
	var fetchErr error
	if signingKey != "" && authOK {
		raw, fetchErr = ghAPIGet(ctx, tokens, apiURL, "user/gpg_keys")
	}
	st.Results = append(st.Results, checkGPGGitHub(signingKey, authOK, raw, fetchErr))
	if RequiredEmailDomain != "" {
		st.Results = append(st.Results, checkEmailDomain(signingKey, authOK, raw, fetchErr))
	}

	return st
}

func checkGitHubAuth(ctx context.Context, tokens auth.TokenStore) (bool, Result) {
	r := Result{Step: StepGitHubAuth, Title: "GitHub authenticated"}
	if ghAvailable() {
		if _, err := runGH(ctx, "auth", "status"); err == nil {
			r.OK, r.Detail = true, "gh CLI is logged in"
			return true, r
		}
	}
	if tok, origin := ghToken(tokens); tok != "" {
		r.OK, r.Detail = true, "using "+origin
		return true, r
	}
	r.Detail = "no working GitHub authentication found"
	r.Fix = []string{"gh auth login", "munin login github"}
	return false, r
}

func checkSigningKey(ctx context.Context) (string, Result) {
	r := Result{Step: StepGitSigningKey, Title: "git commit signing key configured"}
	out, err := runGit(ctx, "config", "--get", "user.signingkey")
	key := strings.TrimSpace(string(out))
	if err != nil || key == "" {
		r.Detail = "git config user.signingkey is not set"
		r.Fix = []string{
			"gpg --full-generate-key",
			"gpg --list-secret-keys --keyid-format=long",
			"git config --global user.signingkey <key-id>",
			"git config --global commit.gpgsign true",
		}
		return "", r
	}
	r.OK, r.Detail = true, "user.signingkey = "+key
	if sign, _ := runGit(ctx, "config", "--get", "commit.gpgsign"); strings.TrimSpace(string(sign)) != "true" {
		r.Detail += " (commit.gpgsign is not enabled: git config --global commit.gpgsign true)"
	}
	return key, r
}

func checkGPGLocal(ctx context.Context, signingKey string) Result {
	r := Result{Step: StepGPGLocal, Title: "GPG secret key present locally"}
	if signingKey == "" {
		r.Detail = "blocked: configure git user.signingkey first"
		return r
	}
	if _, err := runGPG(ctx, "--list-secret-keys", signingKey); err != nil {
		r.Detail = "no local GPG secret key matching " + signingKey
		r.Fix = []string{
			"gpg --list-secret-keys --keyid-format=long",
			"gpg --full-generate-key",
		}
		return r
	}
	r.OK, r.Detail = true, "secret key "+signingKey+" found in your GPG keyring"
	return r
}

func checkGPGGitHub(signingKey string, authOK bool, raw []byte, fetchErr error) Result {
	r := Result{Step: StepGPGGitHub, Title: "GPG key verified on GitHub"}
	switch {
	case signingKey == "":
		r.Detail = "blocked: configure git user.signingkey first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with GitHub first"
		return r
	case fetchErr != nil:
		r.Detail = "could not read your GitHub GPG keys: " + fetchErr.Error()
		r.Fix = uploadFix(signingKey)
		return r
	}
	if !keyRegistered(raw, signingKey) {
		r.Detail = "signing key " + signingKey + " is not registered on your GitHub account"
		r.Fix = uploadFix(signingKey)
		return r
	}
	r.OK, r.Detail = true, "signing key is registered on GitHub; signed commits will show as Verified"
	return r
}

func checkEmailDomain(signingKey string, authOK bool, raw []byte, fetchErr error) Result {
	r := Result{Step: StepEmailDomain, Title: "signing key belongs to @" + RequiredEmailDomain}
	switch {
	case signingKey == "":
		r.Detail = "blocked: configure git user.signingkey first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with GitHub first"
		return r
	case fetchErr != nil:
		r.Detail = "could not read your GitHub GPG keys: " + fetchErr.Error()
		return r
	}
	emails := keyEmails(raw, signingKey)
	for _, e := range emails {
		if emailInDomain(e, RequiredEmailDomain) {
			r.OK, r.Detail = true, "verified identity "+e
			return r
		}
	}
	r.Detail = "the registered signing key has no verified @" + RequiredEmailDomain + " identity on GitHub"
	r.Fix = []string{
		"add and verify your @" + RequiredEmailDomain + " email in GitHub settings",
		"ensure that email is a user id on the GPG key (gpg --edit-key " + signingKey + " adduid), then re-upload the key",
	}
	return r
}

func uploadFix(signingKey string) []string {
	return []string{
		"gpg --armor --export " + signingKey + " | gh gpg-key add -",
		"or: gpg --armor --export " + signingKey + "   then paste at https://github.com/settings/gpg/new",
	}
}

type ghGPGKey struct {
	KeyID  string `json:"key_id"`
	Emails []struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	} `json:"emails"`
	Subkeys []struct {
		KeyID string `json:"key_id"`
	} `json:"subkeys"`
}

func keyEmails(raw []byte, signingKey string) []string {
	var keys []ghGPGKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil
	}
	want := normalizeKeyID(signingKey)
	if want == "" {
		return nil
	}
	var out []string
	for _, k := range keys {
		matched := keyIDMatch(k.KeyID, want)
		for _, sk := range k.Subkeys {
			if keyIDMatch(sk.KeyID, want) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		for _, e := range k.Emails {
			if e.Verified && e.Email != "" {
				out = append(out, e.Email)
			}
		}
	}
	return out
}

func emailInDomain(email, domain string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	return strings.EqualFold(email[at+1:], domain)
}

func keyRegistered(raw []byte, signingKey string) bool {
	var keys []ghGPGKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	want := normalizeKeyID(signingKey)
	if want == "" {
		return false
	}
	for _, k := range keys {
		if keyIDMatch(k.KeyID, want) {
			return true
		}
		for _, sk := range k.Subkeys {
			if keyIDMatch(sk.KeyID, want) {
				return true
			}
		}
	}
	return false
}

func normalizeKeyID(s string) string {
	s = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(s)), " ", "")
	return strings.TrimPrefix(s, "0X")
}

func keyIDMatch(ghKeyID, want string) bool {
	g := normalizeKeyID(ghKeyID)
	if g == "" || want == "" {
		return false
	}
	return g == want || strings.HasSuffix(want, g) || strings.HasSuffix(g, want)
}
