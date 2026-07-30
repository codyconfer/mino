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

		if plugin.SignalEnabled("slack") {
			slackLevel := deck.StatusMuted
			if _, err := auth.SlackToken(a.Tokens, ""); err == nil {
				slackLevel = deck.StatusOK
			}
			info.Services = append(info.Services, deck.ServiceStatus{Name: "slack", Level: slackLevel})
		}

		googleSignals := []string{"calendar", "gmail", "docs", "drive", "tasks"}
		anyGoogle := false
		for _, name := range googleSignals {
			if plugin.SignalEnabled(name) {
				anyGoogle = true
				break
			}
		}
		if anyGoogle {
			googleLevel := deck.StatusMuted
			if auth.GoogleAuthed(a.Tokens) {
				googleLevel = deck.StatusOK
			}
			info.Services = append(info.Services, deck.ServiceStatus{
				ID:    "google",
				Name:  "google",
				Level: googleLevel,
			})
		}
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
