package statusstrip

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/pluginhost"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

type ChipFunc func() (deck.ServiceStatus, bool)

var chips []ChipFunc

func RegisterChip(fn ChipFunc) { chips = append(chips, fn) }

func resetChips() { chips = nil }

func Provider(a *app.App) deck.StatusFunc {
	return func(ctx context.Context) deck.StatusInfo {
		apiURL, _ := gh.NormalizeAPIURL(a.Cfg.GitHub.APIURL)
		var info deck.StatusInfo

		if plugin.SignalEnabled("github") {
			user, rate, ghOK := githubStatus(ctx, a, apiURL)
			info.GitHubUser = user
			info.Services = append(info.Services, rate)

			if ghOK {
				st := onboard.Check(ctx, a.Tokens, apiURL)
				info.SigningVerified = signingVerified(st)
			}
		}

		info.Services = append(info.Services, providerStatuses(a)...)

		if svc, ok := credentialStoreChip(); ok {
			info.Services = append(info.Services, svc)
		}
		for _, chip := range chips {
			if svc, ok := chip(); ok {
				info.Services = append(info.Services, svc)
			}
		}
		home, roleName := a.Cfg.Home, a.Role()
		info.Services = append(info.Services, deck.PluginServicesContext(ctx, home, roleName)...)
		info.Services = append(info.Services, deck.RoleServices()...)
		return info
	}
}

func credentialStoreChip() (deck.ServiceStatus, bool) {
	if auth.CredentialStoreError() == nil {
		return deck.ServiceStatus{}, false
	}
	return deck.ServiceStatus{
		ID:     "credentials",
		Name:   "credentials",
		Detail: auth.CredUnreadable.String(),
		Level:  deck.StatusBad,
	}, true
}

func githubStatus(ctx context.Context, a *app.App, apiURL string) (user string, svc deck.ServiceStatus, ok bool) {
	svc = deck.ServiceStatus{Name: "github"}
	raw, err := auth.GHAPIGet(ctx, a.Tokens, apiURL, "user")
	if err != nil {
		svc.Level = deck.StatusBad
		return "", svc, false
	}
	var u struct {
		Login string `json:"login"`
	}
	_ = json.Unmarshal(raw, &u)

	limit, remaining, rateOK := githubRate(ctx, a, apiURL)
	if !rateOK {
		svc.Level = deck.StatusOK
		return u.Login, svc, true
	}
	svc.Detail = fmt.Sprintf("%d/%d", remaining, limit)
	switch {
	case remaining == 0:
		svc.Level = deck.StatusBad
	case remaining*5 < limit:
		svc.Level = deck.StatusWarn
	default:
		svc.Level = deck.StatusOK
	}
	return u.Login, svc, true
}

func githubRate(ctx context.Context, a *app.App, apiURL string) (limit, remaining int, ok bool) {
	raw, err := auth.GHAPIGet(ctx, a.Tokens, apiURL, "rate_limit")
	if err != nil {
		return 0, 0, false
	}
	var r struct {
		Resources struct {
			Core struct {
				Limit     int `json:"limit"`
				Remaining int `json:"remaining"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || r.Resources.Core.Limit == 0 {
		return 0, 0, false
	}
	return r.Resources.Core.Limit, r.Resources.Core.Remaining, true
}

func signingVerified(st onboard.Status) bool {
	for _, r := range st.Results {
		if r.Step == onboard.StepGPGGitHub || r.Step == onboard.StepSSHGitHub {
			return r.OK
		}
	}
	return false
}

func providerStatuses(a *app.App) []deck.ServiceStatus {
	host := pluginhost.New(a.Cfg, a.Tokens)
	var out []deck.ServiceStatus
	for _, p := range plugin.LoginProviders() {
		if !providerEnabled(p) {
			continue
		}
		level := deck.StatusMuted
		if p.Authed != nil && p.Authed(host) {
			level = deck.StatusOK
		}
		out = append(out, deck.ServiceStatus{ID: p.Key, Name: p.Key, Level: level})
	}
	return out
}

func providerEnabled(p plugin.LoginProvider) bool {
	names := p.Signals
	if len(names) == 0 {
		names = []string{p.Key}
	}
	for _, name := range names {
		if plugin.SignalEnabled(name) {
			return true
		}
	}
	return false
}
