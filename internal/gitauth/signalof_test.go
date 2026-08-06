package gitauth

import (
	"errors"
	"fmt"
	"testing"
)

type aliasProvider struct {
	*stubProvider
	signal string
}

func (a *aliasProvider) Signal() string { return a.signal }

func TestSignalOfPrefersTheDeclaredSignal(t *testing.T) {
	cases := []struct {
		name string
		p    Provider
		want string
	}{
		{"plain provider reports itself", &stubProvider{name: "github"}, "github"},
		{"alias reports the shared signal", &aliasProvider{stubProvider: &stubProvider{name: "forgejo"}, signal: "gitea"}, "gitea"},
		{"empty declaration falls back", &aliasProvider{stubProvider: &stubProvider{name: "gitea"}, signal: ""}, "gitea"},
		{"nil provider", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SignalOf(c.p); got != c.want {
				t.Errorf("SignalOf() = %q, want %q; the wrong name gates the status chip on a signal that was never registered", got, c.want)
			}
		})
	}
}

func TestErrRateLimitUnreportedSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("gitea: rate limit: %w", ErrRateLimitUnreported)
	if !errors.Is(err, ErrRateLimitUnreported) {
		t.Error("a wrapped ErrRateLimitUnreported was not recognised; callers would render 0/0 and claim the user is throttled")
	}
}
