package statusstrip

import (
	"context"
	"fmt"

	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/onboard"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/gitauth"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/pluginhost"
)

type ChipFunc func() (deck.ServiceStatus, bool)

var chips []ChipFunc

func RegisterChip(fn ChipFunc) { chips = append(chips, fn) }

func resetChips() { chips = nil }

func Provider(a *app.App) deck.StatusFunc {
	return func(ctx context.Context) deck.StatusInfo {
		prov, id, _ := a.GitAuth()
		var info deck.StatusInfo

		if plugin.SignalEnabled("github") {
			user, rate, ghOK := providerStatus(ctx, prov, id)
			info.GitHubUser = user
			info.Services = append(info.Services, rate)

			if ghOK {
				st := onboard.Check(ctx, prov, id)
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
		ID:       "credentials",
		Name:     "credentials",
		Detail:   auth.CredUnreadable.String(),
		Severity: vkglyph.SeverityNegative,
	}, true
}

func providerStatus(ctx context.Context, p gitauth.Provider, id gitauth.Identity) (user string, svc deck.ServiceStatus, ok bool) {
	name := "git"
	if p != nil {
		name = p.Name()
	}
	svc = deck.ServiceStatus{Name: name}
	if p == nil || id == nil {
		svc.Severity = vkglyph.SeverityNegative
		return "", svc, false
	}
	acct, err := p.Account(ctx, id)
	if err != nil {
		svc.Severity = vkglyph.SeverityNegative
		return "", svc, false
	}
	rl, rerr := p.RateLimit(ctx, id)
	if rerr != nil {
		svc.Severity = vkglyph.SeverityPositive
		return acct.Login, svc, true
	}
	svc.Detail = fmt.Sprintf("%d/%d", rl.Remaining, rl.Limit)
	switch {
	case rl.Remaining == 0:
		svc.Severity = vkglyph.SeverityNegative
	case rl.Remaining*5 < rl.Limit:
		svc.Severity = vkglyph.SeverityWarning
	default:
		svc.Severity = vkglyph.SeverityPositive
	}
	return acct.Login, svc, true
}

func signingVerified(st onboard.Status) bool {
	for _, r := range st.Results {
		if r.Step == onboard.StepGPGRemote || r.Step == onboard.StepSSHRemote {
			return r.OK
		}
	}
	return false
}

func providerStatuses(a *app.App) []deck.ServiceStatus {
	var out []deck.ServiceStatus
	for _, p := range plugin.LoginProviders() {
		if !providerEnabled(p) {
			continue
		}
		level := vkglyph.SeverityNeutral
		if p.Authed != nil && p.Authed(pluginhost.ForLogin(a.Cfg, a.Tokens, a.Role(), p)) {
			level = vkglyph.SeverityPositive
		}
		out = append(out, deck.ServiceStatus{ID: p.Key, Name: p.Key, Severity: level})
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
