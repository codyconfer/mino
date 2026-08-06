package auth

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func TestNormalizeGitHubAPIURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"https default", "https://api.github.com", "https://api.github.com", false},
		{"https trailing slash", "https://ghe.example.com/api/v3/", "https://ghe.example.com/api/v3", false},
		{"http rejected", "http://evil.example/api/v3", "", true},
		{"loopback http rejected", "http://localhost:3000/api/v3", "", true},
		{"scheme-less rejected", "api.github.com", "", true},
		{"no host rejected", "https://", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeGitHubAPIURL(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NormalizeGitHubAPIURL(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("NormalizeGitHubAPIURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeGitHubAPIURLKeepsItsOwnFieldNames(t *testing.T) {
	_, err := NormalizeGitHubAPIURL("http://evil.example/api/v3")
	if err == nil {
		t.Fatal("http was accepted")
	}
	if !strings.Contains(err.Error(), "github: api_url must use https") {
		t.Errorf("message = %q, want it to name github's api_url field", err.Error())
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "github.api_url") {
		t.Errorf("hint = %q, want it to name github.api_url", hint)
	}
}

func TestNormalizeGiteaURLAllowsLoopbackHTTP(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"https root", "https://git.example.com", "https://git.example.com", false},
		{"https trailing slash", "https://git.example.com/", "https://git.example.com", false},
		{"subpath install", "https://example.com/gitea", "https://example.com/gitea", false},
		{"localhost http", "http://localhost:3000", "http://localhost:3000", false},
		{"loopback ip http", "http://127.0.0.1:3000", "http://127.0.0.1:3000", false},
		{"ipv6 loopback http", "http://[::1]:3000", "http://[::1]:3000", false},
		{"remote http rejected", "http://git.example.com", "", true},
		{"scheme-less rejected", "git.example.com", "", true},
		{"no host rejected", "https://", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeGiteaURL(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NormalizeGiteaURL(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("NormalizeGiteaURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeGiteaURLNamesGiteaFields(t *testing.T) {
	_, err := NormalizeGiteaURL("http://git.example.com")
	if err == nil {
		t.Fatal("remote http was accepted")
	}
	if !strings.Contains(err.Error(), "gitea: url must use https") {
		t.Errorf("message = %q, want it to name gitea's url field", err.Error())
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "gitea.url") {
		t.Errorf("hint = %q, want it to name gitea.url, not github.api_url", hint)
	}

	_, err = NormalizeGiteaAPIURL("http://git.example.com/api/v1")
	if err == nil {
		t.Fatal("remote http api_url was accepted")
	}
	if hint := errs.Hint(err); !strings.Contains(hint, "gitea.api_url") {
		t.Errorf("hint = %q, want it to name gitea.api_url", hint)
	}
}
