package onboard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/codyconfer/munin/internal/auth"
)

var RequiredEmailDomain string

var AllOrNothingAuth string

type StepID string

const (
	StepGitHubAuth    StepID = "github-auth"
	StepGitSigningKey StepID = "git-signingkey"
	StepGPGLocal      StepID = "gpg-local"
	StepGPGGitHub     StepID = "gpg-github"
	StepSSHLocal      StepID = "ssh-local"
	StepSSHGitHub     StepID = "ssh-github"
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
	ghAvailable  = auth.GHAvailable
	ghToken      = auth.GitHubToken
	runGH        = auth.GH
	runGit       = auth.Git
	runGPG       = auth.GPG
	runSSHKeygen = auth.SSHKeygen
	ghAPIGet     = auth.GHAPIGet
	readFile     = os.ReadFile
)

func Check(ctx context.Context, tokens auth.TokenStore, apiURL string) Status {
	var st Status

	authOK, authRes := checkGitHubAuth(ctx, tokens)
	st.Results = append(st.Results, authRes)

	format := signingFormat(ctx)
	signingKey, keyRes := checkSigningKey(ctx, format)
	st.Results = append(st.Results, keyRes)

	if format == "ssh" {
		return checkSSH(ctx, tokens, apiURL, authOK, signingKey, st)
	}
	return checkGPG(ctx, tokens, apiURL, authOK, signingKey, st)
}

func checkGPG(ctx context.Context, tokens auth.TokenStore, apiURL string, authOK bool, signingKey string, st Status) Status {
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

func checkSSH(ctx context.Context, tokens auth.TokenStore, apiURL string, authOK bool, signingKey string, st Status) Status {
	pubKey, localRes := checkSSHLocal(ctx, signingKey)
	st.Results = append(st.Results, localRes)

	var raw []byte
	var fetchErr error
	if pubKey != "" && authOK {
		raw, fetchErr = ghAPIGet(ctx, tokens, apiURL, "user/ssh_signing_keys")
	}
	st.Results = append(st.Results, checkSSHGitHub(pubKey, authOK, raw, fetchErr))
	if RequiredEmailDomain != "" {
		st.Results = append(st.Results, checkSSHEmailDomain(ctx, tokens, apiURL, authOK))
	}

	return st
}

func signingFormat(ctx context.Context) string {
	out, err := runGit(ctx, "config", "--get", "gpg.format")
	if err != nil {
		return "openpgp"
	}
	if strings.ToLower(strings.TrimSpace(string(out))) == "ssh" {
		return "ssh"
	}
	return "openpgp"
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

func checkSigningKey(ctx context.Context, format string) (string, Result) {
	r := Result{Step: StepGitSigningKey, Title: "git commit signing key configured"}
	out, err := runGit(ctx, "config", "--get", "user.signingkey")
	key := strings.TrimSpace(string(out))
	if err != nil || key == "" {
		r.Detail = "git config user.signingkey is not set"
		if format == "ssh" {
			r.Fix = []string{
				"ssh-keygen -t ed25519 -C \"you@example.com\"",
				"git config --global user.signingkey ~/.ssh/id_ed25519.pub",
				"git config --global commit.gpgsign true",
			}
		} else {
			r.Fix = []string{
				"gpg --full-generate-key",
				"gpg --list-secret-keys --keyid-format=long",
				"git config --global user.signingkey <key-id>",
				"git config --global commit.gpgsign true",
			}
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
		if fix := scopeFix(fetchErr, "admin:gpg_key"); fix != nil {
			r.Detail = "cannot read your GitHub GPG keys: your GitHub token is missing the admin:gpg_key scope"
			r.Fix = fix
			return r
		}
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

func scopeFix(err error, scope string) []string {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, strings.ToLower(scope)) ||
		strings.Contains(s, "404") ||
		strings.Contains(s, "not found") {
		return []string{"gh auth refresh -h github.com -s " + scope}
	}
	return nil
}

func uploadFix(signingKey string) []string {
	return []string{
		"gpg --armor --export " + signingKey + " | gh gpg-key add -",
		"or: gpg --armor --export " + signingKey + "   then paste at https://github.com/settings/gpg/new",
	}
}

func checkSSHLocal(ctx context.Context, signingKey string) (string, Result) {
	r := Result{Step: StepSSHLocal, Title: "SSH signing key present locally"}
	if signingKey == "" {
		r.Detail = "blocked: configure git user.signingkey first"
		return "", r
	}
	pub, err := resolveSSHPublicKey(ctx, signingKey)
	if err != nil || pub == "" {
		r.Detail = "could not read the SSH signing key from " + signingKey
		r.Fix = []string{
			"point user.signingkey at your SSH public or private key",
			"ssh-keygen -y -f ~/.ssh/id_ed25519 > ~/.ssh/id_ed25519.pub",
		}
		return "", r
	}
	r.OK, r.Detail = true, "using SSH "+sshKeyType(pub)+" signing key"
	return pub, r
}

func checkSSHGitHub(pubKey string, authOK bool, raw []byte, fetchErr error) Result {
	r := Result{Step: StepSSHGitHub, Title: "SSH signing key registered on GitHub"}
	switch {
	case pubKey == "":
		r.Detail = "blocked: configure a local SSH signing key first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with GitHub first"
		return r
	case fetchErr != nil:
		if fix := scopeFix(fetchErr, "admin:ssh_signing_key"); fix != nil {
			r.Detail = "cannot read your GitHub SSH signing keys: your GitHub token is missing the admin:ssh_signing_key scope"
			r.Fix = fix
			return r
		}
		r.Detail = "could not read your GitHub SSH signing keys: " + fetchErr.Error()
		r.Fix = sshUploadFix()
		return r
	}
	if !sshKeyRegistered(raw, pubKey) {
		r.Detail = "this SSH key is not registered as a signing key on your GitHub account"
		r.Fix = sshUploadFix()
		return r
	}
	r.OK, r.Detail = true, "SSH signing key is registered on GitHub; signed commits will show as Verified"
	return r
}

func checkSSHEmailDomain(ctx context.Context, tokens auth.TokenStore, apiURL string, authOK bool) Result {
	r := Result{Step: StepEmailDomain, Title: "commit email belongs to @" + RequiredEmailDomain}
	if !authOK {
		r.Detail = "blocked: authenticate with GitHub first"
		return r
	}
	out, err := runGit(ctx, "config", "--get", "user.email")
	email := strings.TrimSpace(string(out))
	if err != nil || email == "" {
		r.Detail = "git config user.email is not set"
		r.Fix = []string{"git config --global user.email you@" + RequiredEmailDomain}
		return r
	}
	if !emailInDomain(email, RequiredEmailDomain) {
		r.Detail = "user.email " + email + " is not an @" + RequiredEmailDomain + " address"
		r.Fix = []string{"git config --global user.email you@" + RequiredEmailDomain}
		return r
	}
	raw, ferr := ghAPIGet(ctx, tokens, apiURL, "user/emails")
	if ferr != nil {
		r.Detail = "could not read your GitHub emails: " + ferr.Error()
		return r
	}
	if !emailVerifiedOnGitHub(raw, email) {
		r.Detail = email + " is not a verified email on your GitHub account"
		r.Fix = []string{"add and verify " + email + " in GitHub email settings"}
		return r
	}
	r.OK, r.Detail = true, "verified identity "+email
	return r
}

func sshUploadFix() []string {
	return []string{
		"gh ssh-key add <path-to-.pub> --type signing",
		"or paste the key at https://github.com/settings/ssh/new (Key type: Signing Key)",
	}
}

func resolveSSHPublicKey(ctx context.Context, signingKey string) (string, error) {
	s := strings.TrimSpace(signingKey)
	if lit, ok := strings.CutPrefix(s, "key::"); ok {
		return normalizeSSHKey(lit), nil
	}
	if isSSHKeyLiteral(s) {
		return normalizeSSHKey(s), nil
	}
	path := expandPath(s)
	data, readErr := readFile(path)
	if readErr == nil && isSSHKeyLiteral(string(data)) {
		return normalizeSSHKey(string(data)), nil
	}
	out, keygenErr := runSSHKeygen(ctx, "-y", "-f", path)
	if keygenErr != nil {
		if readErr != nil {
			return "", readErr
		}
		return "", keygenErr
	}
	return normalizeSSHKey(string(out)), nil
}

func isSSHKeyLiteral(s string) bool {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return false
	}
	switch {
	case strings.HasPrefix(f[0], "ssh-"),
		strings.HasPrefix(f[0], "sk-ssh-"),
		strings.HasPrefix(f[0], "ecdsa-"),
		strings.HasPrefix(f[0], "sk-ecdsa-"):
		return true
	}
	return false
}

func normalizeSSHKey(s string) string {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return ""
	}
	return f[0] + " " + f[1]
}

func sshKeyType(pubKey string) string {
	f := strings.Fields(pubKey)
	if len(f) == 0 {
		return "public"
	}
	return f[0]
}

func expandPath(p string) string {
	if p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

type ghSSHKey struct {
	Key string `json:"key"`
}

func sshKeyRegistered(raw []byte, pubKey string) bool {
	var keys []ghSSHKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	want := normalizeSSHKey(pubKey)
	if want == "" {
		return false
	}
	for _, k := range keys {
		if normalizeSSHKey(k.Key) == want {
			return true
		}
	}
	return false
}

func emailVerifiedOnGitHub(raw []byte, email string) bool {
	var list []struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return false
	}
	for _, e := range list {
		if e.Verified && strings.EqualFold(e.Email, email) {
			return true
		}
	}
	return false
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
	s = strings.TrimSuffix(s, "!")
	return strings.TrimPrefix(s, "0X")
}

func keyIDMatch(ghKeyID, want string) bool {
	g := normalizeKeyID(ghKeyID)
	if g == "" || want == "" {
		return false
	}
	return g == want || strings.HasSuffix(want, g) || strings.HasSuffix(g, want)
}
