package loginflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func TestResolveAcceptsTheForgejoAlias(t *testing.T) {
	for _, name := range []string{"gitea", "forgejo"} {
		p, ok := Resolve(name)
		if !ok {
			t.Fatalf("Resolve(%q) found no provider", name)
		}
		if p.Key != "gitea" {
			t.Errorf("Resolve(%q).Key = %q, want gitea; both names share one credential", name, p.Key)
		}
	}
	if names := strings.Join(Names(), " "); !strings.Contains(names, "forgejo") {
		t.Errorf("Names() = %q, want the forgejo alias so completion and ValidArgs offer it", names)
	}
}

func TestGiteaLoginPromptsForATokenNotAnOAuthClientID(t *testing.T) {
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Gitea.URL = "https://git.example.com"
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)
	t.Setenv("GITEA_TOKEN", "")
	t.Setenv("FORGEJO_TOKEN", "")

	p, _ := Resolve("gitea")
	miss := p.Missing(a)
	if len(miss) != 1 || miss[0].Key != "gitea.token" {
		t.Fatalf("Missing() = %#v, want just the access token", miss)
	}
	if !miss[0].Secret || !miss[0].Sealed {
		t.Errorf("field = %+v, want it secret and sealed", miss[0])
	}
}

func TestGiteaLoginPromptsNothingOnceATokenResolves(t *testing.T) {
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Gitea.URL = "https://git.example.com"
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)
	t.Setenv("GITEA_TOKEN", "ambient-tok")

	p, _ := Resolve("gitea")
	if miss := p.Missing(a); len(miss) != 0 {
		t.Errorf("Missing() = %#v, want nothing: $GITEA_TOKEN already resolves", miss)
	}
	for _, f := range p.Fields {
		if got := f.Cur(a); strings.Contains(got, "ambient-tok") {
			t.Errorf("Cur() = %q, want the origin rather than the token; login menus render this", got)
		}
	}
}

func TestSealedFieldsArePromptedButNeverPersisted(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Home = dir
	a := &app.App{Cfg: cfg}
	t.Cleanup(a.CloseDBs)

	p := Provider{
		Key: "probe",
		Fields: []CredField{
			{Key: "probe.oauth_client_id", Label: "client id"},
			{Key: "probe.token", Label: "token", Secret: true, Sealed: true},
		},
	}
	values := map[string]string{"probe.oauth_client_id": "public", "probe.token": "gta_secret"}

	keep := p.Persistable(values)
	if keep["probe.oauth_client_id"] != "public" {
		t.Errorf("Persistable dropped a persistable field: %#v", keep)
	}
	if _, ok := keep["probe.token"]; ok {
		t.Fatal("a sealed field survived Persistable; it would be written to config.yaml and reflected back out of /api/v1/config")
	}

	if err := PersistCredentials(a, keep); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "gta_secret") {
		t.Errorf("config.yaml contains the sealed token:\n%s", raw)
	}
}
