package deck

import (
	"context"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render/glyph"
)

// StatusLevel is glyph.Severity for status chips (single severity vocabulary).
type StatusLevel = vkglyph.Severity

const (
	StatusMuted = vkglyph.SeverityNeutral
	StatusOK    = vkglyph.SeverityPositive
	StatusWarn  = vkglyph.SeverityWarning
	StatusBad   = vkglyph.SeverityNegative
)

// ServiceStatus is one right-strip status chip (munin-facing).
type ServiceStatus struct {
	Name   string
	Detail string
	Level  StatusLevel
	// Glyph optionally overrides theme.SeverityGlyph(Level) (plugin contribs).
	Glyph string
}

// StatusInfo is munin's chrome identity payload.
type StatusInfo struct {
	GitHubUser      string
	SigningVerified bool
	Services        []ServiceStatus
}

// StatusFunc loads status asynchronously for the chrome strip.
type StatusFunc func(context.Context) StatusInfo

// PluginServices converts enabled plugin status contributions into chrome chips.
func PluginServices(home, role string) []ServiceStatus {
	contribs := plugin.CollectStatusContributions(home, role)
	out := make([]ServiceStatus, 0, len(contribs))
	for _, c := range contribs {
		if c.Status == nil {
			continue
		}
		g, tone := c.Status()
		if g == "" {
			continue
		}
		name := ""
		if c.Info != nil {
			name = c.Info()
		}
		out = append(out, ServiceStatus{Name: name, Level: tone, Glyph: g})
	}
	return out
}

func adaptStatus(info StatusInfo) vkdeck.StatusInfo {
	out := vkdeck.StatusInfo{Identity: identity(info)}
	for _, s := range info.Services {
		g := s.Glyph
		if g == "" {
			g = theme.SeverityGlyph(s.Level)
		}
		out.Services = append(out.Services, vkdeck.ServiceStatus{
			Name:   s.Name,
			Detail: s.Detail,
			Glyph:  glyph.Lead(g),
			Color:  theme.SeverityColor(s.Level),
		})
	}
	return out
}

func identity(info StatusInfo) string {
	if info.GitHubUser == "" {
		return ""
	}
	th := theme.Cur()
	mark := theme.StripText(th.Cant.GetForeground(), glyph.SigningBad())
	if info.SigningVerified {
		mark = theme.StripText(th.Can.GetForeground(), glyph.SigningOK())
	}
	return theme.StripBold(th.Key.GetForeground(), "@"+info.GitHubUser) +
		theme.StripText(th.Dim.GetForeground(), " ") + mark
}
