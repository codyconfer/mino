package cmd

import (
	"testing"

	"github.com/codyconfer/mino/internal/config"
)

func TestDetailSignalForRoutesByHost(t *testing.T) {
	cfg := config.Defaults()
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"gitlab.com", "https://gitlab.com/acme/api/-/merge_requests/42", "gitlab"},
		{"gitlab subdomain", "https://gitlab.acme.io/acme/api/-/issues/7", "gitlab"},
		{"github.com", "https://github.com/owner/repo/pull/123", "github"},
		{"github subdomain", "https://github.acme.io/owner/repo/pull/1", "github"},
		{"unknown host", "https://bitbucket.org/owner/repo/pull-requests/1", defaultDetailSignal},
		{"not a url", "::nonsense", defaultDetailSignal},
		{"empty", "", defaultDetailSignal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detailSignalFor(c.url, cfg); got != c.want {
				t.Errorf("detailSignalFor(%q) = %q, want %q; without routing a GitLab URL reaches "+
					"github's ParseRef and dies telling the user to pass a GitHub URL", c.url, got, c.want)
			}
		})
	}
}

func TestDetailSignalForPrefersConfiguredEndpoints(t *testing.T) {
	cfg := config.Defaults()
	cfg.GitLab.APIURL = "https://git.acme.internal"
	cfg.GitHub.APIURL = "https://ghe.acme.internal/api/v3"

	if got := detailSignalFor("https://git.acme.internal/g/p/-/issues/3", cfg); got != "gitlab" {
		t.Errorf("self-managed gitlab host = %q, want gitlab; a host that names neither forge can "+
			"only be recognised from the configured endpoint", got)
	}
	if got := detailSignalFor("https://ghe.acme.internal/o/r/pull/3", cfg); got != "github" {
		t.Errorf("self-managed github host = %q, want github", got)
	}
	if got := detailSignalFor("https://elsewhere.internal/o/r/pull/3", cfg); got != defaultDetailSignal {
		t.Errorf("unknown host = %q, want the default", got)
	}
}

func TestDetailSignalForIgnoresPortsAndCase(t *testing.T) {
	cfg := config.Defaults()
	cfg.GitLab.APIURL = "https://GIT.acme.internal:8443/api/v4"

	if got := detailSignalFor("https://git.acme.internal:8443/g/p/-/merge_requests/1", cfg); got != "gitlab" {
		t.Errorf("host with a port = %q, want gitlab", got)
	}
	if got := detailSignalFor("https://GitLab.com/g/p/-/issues/1", cfg); got != "gitlab" {
		t.Errorf("mixed-case host = %q, want gitlab", got)
	}
}

func TestDetailSignalForFallsBackWhenTheSignalHasNoDetails(t *testing.T) {
	if got := enabledDetailSignal("nosuchsignal"); got != defaultDetailSignal {
		t.Errorf("enabledDetailSignal(unknown) = %q, want the default; routing to a signal that "+
			"cannot render a detail would turn a working command into an error", got)
	}
	if got := enabledDetailSignal("gitlab"); got != "gitlab" {
		t.Errorf("enabledDetailSignal(gitlab) = %q, want gitlab", got)
	}
}

func TestDetailSignalForToleratesANilConfig(t *testing.T) {
	if got := detailSignalFor("https://gitlab.com/g/p/-/issues/1", nil); got != "gitlab" {
		t.Errorf("with no config = %q, want gitlab from the well-known host", got)
	}
}
