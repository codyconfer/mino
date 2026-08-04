package onboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/gitauth"
)

var RequiredEmailDomain string

var AllOrNothingAuth string

var ServiceAuth string

func ServiceAuthAllowed() bool { return ServiceAuth == "true" }

type StepID string

const (
	StepProviderAuth  StepID = "provider-auth"
	StepGitSigningKey StepID = "git-signingkey"
	StepGPGLocal      StepID = "gpg-local"
	StepGPGRemote     StepID = "gpg-remote"
	StepSSHLocal      StepID = "ssh-local"
	StepSSHRemote     StepID = "ssh-remote"
	StepEmailDomain   StepID = "email-domain"
	StepServiceID     StepID = "service-identity"
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
	runGH        = auth.GH
	runGit       = auth.Git
	runGPG       = auth.GPG
	runSSHKeygen = auth.SSHKeygen
	readFile     = os.ReadFile
)

func Check(ctx context.Context, p gitauth.Provider, id gitauth.Identity) Status {
	var st Status

	authOK, authRes := checkProviderAuth(ctx, p, id)
	st.Results = append(st.Results, authRes)

	if authOK && ServiceAuthAllowed() && id != nil && id.ServiceIdentity() {
		st.Results = append(st.Results, Result{
			Step:   StepServiceID,
			Title:  "running as a service identity",
			OK:     true,
			Detail: "commit-signing checks do not apply to " + id.Origin(),
		})
		return st
	}

	format := signingFormat(ctx)
	signingKey, keyRes := checkSigningKey(ctx, format)
	st.Results = append(st.Results, keyRes)

	if format == "ssh" {
		return checkSSH(ctx, p, id, authOK, signingKey, st)
	}
	return checkGPG(ctx, p, id, authOK, signingKey, st)
}

func checkGPG(ctx context.Context, p gitauth.Provider, id gitauth.Identity, authOK bool, signingKey string, st Status) Status {
	st.Results = append(st.Results, checkGPGLocal(ctx, signingKey))

	var chk gitauth.KeyCheck
	if signingKey != "" && authOK {
		chk = p.SigningKeyRegistered(ctx, id, gitauth.SigningGPG, signingKey)
	}
	st.Results = append(st.Results, checkGPGRemote(p, signingKey, authOK, chk))
	if RequiredEmailDomain != "" {
		st.Results = append(st.Results, checkEmailDomain(ctx, p, id, signingKey, authOK))
	}

	return st
}

func checkSSH(ctx context.Context, p gitauth.Provider, id gitauth.Identity, authOK bool, signingKey string, st Status) Status {
	pubKey, localRes := checkSSHLocal(ctx, signingKey)
	st.Results = append(st.Results, localRes)

	var chk gitauth.KeyCheck
	if pubKey != "" && authOK {
		chk = p.SigningKeyRegistered(ctx, id, gitauth.SigningSSH, pubKey)
	}
	st.Results = append(st.Results, checkSSHRemote(p, pubKey, authOK, chk))
	if RequiredEmailDomain != "" {
		st.Results = append(st.Results, checkSSHEmailDomain(ctx, p, id, authOK))
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

func checkProviderAuth(ctx context.Context, p gitauth.Provider, id gitauth.Identity) (bool, Result) {
	r := Result{Step: StepProviderAuth, Title: providerLabel(p) + " authenticated"}
	if p == nil {
		r.Detail = "no git provider configured"
		return false, r
	}
	st := providerStatus(ctx, p, id)
	r.OK, r.Detail, r.Fix = st.OK, st.Detail, st.Fix
	return st.OK, r
}

func providerStatus(ctx context.Context, p gitauth.Provider, id gitauth.Identity) gitauth.AuthStatus {
	return p.Status(ctx, id)
}

func providerLabel(p gitauth.Provider) string {
	if p == nil {
		return "git provider"
	}
	return p.Label()
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

func checkGPGRemote(p gitauth.Provider, signingKey string, authOK bool, chk gitauth.KeyCheck) Result {
	label := providerLabel(p)
	r := Result{Step: StepGPGRemote, Title: "GPG key verified on " + label}
	switch {
	case signingKey == "":
		r.Detail = "blocked: configure git user.signingkey first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with " + label + " first"
		return r
	case chk.Err != nil:
		if len(chk.Fix) > 0 {
			r.Detail = "cannot read your " + label + " GPG keys: the credential lacks permission"
			r.Fix = chk.Fix
			return r
		}
		r.Detail = "could not read your " + label + " GPG keys: " + chk.Err.Error()
		r.Fix = p.UploadKeyFix(gitauth.SigningGPG, signingKey)
		return r
	}
	if !chk.Registered {
		r.Detail = "signing key " + signingKey + " is not registered on your " + label + " account"
		r.Fix = p.UploadKeyFix(gitauth.SigningGPG, signingKey)
		return r
	}
	r.OK, r.Detail = true, "signing key is registered on "+label+"; signed commits will show as Verified"
	return r
}

func checkEmailDomain(ctx context.Context, p gitauth.Provider, id gitauth.Identity, signingKey string, authOK bool) Result {
	label := providerLabel(p)
	r := Result{Step: StepEmailDomain, Title: "signing key belongs to @" + RequiredEmailDomain}
	switch {
	case signingKey == "":
		r.Detail = "blocked: configure git user.signingkey first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with " + label + " first"
		return r
	}
	chk := p.SigningKeyRegistered(ctx, id, gitauth.SigningGPG, signingKey)
	if chk.Err != nil {
		r.Detail = "could not read your " + label + " GPG keys: " + chk.Err.Error()
		return r
	}
	for _, e := range chk.Identities {
		if emailInDomain(e, RequiredEmailDomain) {
			r.OK, r.Detail = true, "verified identity "+e
			return r
		}
	}
	r.Detail = "the registered signing key has no verified @" + RequiredEmailDomain + " identity on " + label
	r.Fix = []string{
		"add and verify your @" + RequiredEmailDomain + " email in " + label + " settings",
		"ensure that email is a user id on the GPG key (gpg --edit-key " + signingKey + " adduid), then re-upload the key",
	}
	return r
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

func checkSSHRemote(p gitauth.Provider, pubKey string, authOK bool, chk gitauth.KeyCheck) Result {
	label := providerLabel(p)
	r := Result{Step: StepSSHRemote, Title: "SSH signing key registered on " + label}
	switch {
	case pubKey == "":
		r.Detail = "blocked: configure a local SSH signing key first"
		return r
	case !authOK:
		r.Detail = "blocked: authenticate with " + label + " first"
		return r
	case chk.Err != nil:
		if len(chk.Fix) > 0 {
			r.Detail = "cannot read your " + label + " SSH signing keys: the credential lacks permission"
			r.Fix = chk.Fix
			return r
		}
		r.Detail = "could not read your " + label + " SSH signing keys: " + chk.Err.Error()
		r.Fix = p.UploadKeyFix(gitauth.SigningSSH, pubKey)
		return r
	}
	if !chk.Registered {
		r.Detail = "this SSH key is not registered as a signing key on your " + label + " account"
		r.Fix = p.UploadKeyFix(gitauth.SigningSSH, pubKey)
		return r
	}
	r.OK, r.Detail = true, "SSH signing key is registered on "+label+"; signed commits will show as Verified"
	return r
}

func checkSSHEmailDomain(ctx context.Context, p gitauth.Provider, id gitauth.Identity, authOK bool) Result {
	label := providerLabel(p)
	r := Result{Step: StepEmailDomain, Title: "commit email belongs to @" + RequiredEmailDomain}
	if !authOK {
		r.Detail = "blocked: authenticate with " + label + " first"
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
	verified, verr := p.EmailVerified(ctx, id, email)
	if verr != nil {
		r.Detail = "could not read your " + label + " emails: " + verr.Error()
		return r
	}
	if !verified {
		r.Detail = "user.email " + email + " is not a verified email on your " + label + " account"
		r.Fix = []string{"add and verify " + email + " in " + label + " settings"}
		return r
	}
	r.OK, r.Detail = true, "verified commit email "+email
	return r
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

func emailInDomain(email, domain string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	return strings.EqualFold(email[at+1:], domain)
}
