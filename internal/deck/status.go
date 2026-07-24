package deck

import (
	"context"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
)

// StatusLevel aliases theme severity for status chips.
type StatusLevel = theme.Severity

const (
	StatusMuted = theme.SevMuted
	StatusOK    = theme.SevOK
	StatusWarn  = theme.SevWarn
	StatusBad   = theme.SevBad
)

// ServiceStatus is one right-strip status chip (munin-facing).
type ServiceStatus struct {
	Name   string
	Detail string
	Level  StatusLevel
}

// StatusInfo is munin's chrome identity payload.
type StatusInfo struct {
	GitHubUser      string
	SigningVerified bool
	Services        []ServiceStatus
}

// StatusFunc loads status asynchronously for the chrome strip.
type StatusFunc func(context.Context) StatusInfo

func adaptStatus(info StatusInfo) vkdeck.StatusInfo {
	out := vkdeck.StatusInfo{Identity: identity(info)}
	for _, s := range info.Services {
		out.Services = append(out.Services, vkdeck.ServiceStatus{
			Name:   s.Name,
			Detail: s.Detail,
			Glyph:  glyph.Lead(theme.SeverityGlyph(s.Level)),
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
