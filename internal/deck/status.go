package deck

import (
	"context"
	"fmt"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/role"
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
	// ID is the stable hide-preference key (plugin id or builtin name).
	// When empty, Name is used.
	ID     string
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
func PluginServices(home, roleName string) []ServiceStatus {
	entries := plugin.CollectStatusEntries(home, roleName)
	out := make([]ServiceStatus, 0, len(entries))
	for _, e := range entries {
		c := e.Contrib
		if c.Status == nil {
			continue
		}
		g, tone := c.Status()
		if g == "" {
			continue
		}
		name := c.BrandGlyph
		if name == "" && c.Info != nil {
			name = c.Info()
		}
		out = append(out, ServiceStatus{ID: e.PluginID, Name: name, Level: tone, Glyph: g})
	}
	return out
}

// RoleServices converts the active role's status blocks into chrome chips.
func RoleServices() []ServiceStatus {
	chips := role.StatusChips()
	if len(chips) == 0 {
		return nil
	}
	out := make([]ServiceStatus, 0, len(chips))
	for _, c := range chips {
		out = append(out, ServiceStatus{
			ID:     fmt.Sprintf("role-status-%d", c.Index),
			Name:   c.Glyph,
			Detail: c.Text,
			Level:  StatusOK,
		})
	}
	return out
}

func adaptStatus(info StatusInfo) vkdeck.StatusInfo {
	out := vkdeck.StatusInfo{Identity: identity(info)}
	for _, s := range info.Services {
		if statusBarHidden(s) {
			continue
		}
		g := s.Glyph
		if g == "" {
			g = theme.SeverityGlyph(s.Level)
		}
		out.Services = append(out.Services, vkdeck.ServiceStatus{
			Name:   serviceLabel(s.Name),
			Detail: s.Detail,
			Glyph:  glyph.Lead(g),
			Color:  theme.SeverityColor(s.Level),
		})
	}
	return out
}

func statusBarHidden(s ServiceStatus) bool {
	id := s.ID
	if id == "" {
		id = s.Name
	}
	return config.StatusBarHidden(id)
}

// serviceLabel prefers a tool logo when one exists; otherwise keeps the text name.
func serviceLabel(name string) string {
	if logo := glyph.ForTool(name); logo != "" {
		return logo
	}
	return name
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
