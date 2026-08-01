package deck

import (
	"context"
	"fmt"
	"time"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/role"
)

// ServiceStatus is viewkit's chip type; severity uses glyph.Severity directly.
type ServiceStatus = vkdeck.ServiceStatus

type StatusInfo struct {
	GitHubUser      string
	SigningVerified bool
	Services        []ServiceStatus
}

type StatusFunc func(context.Context) StatusInfo

const statusContribBudget = 2 * time.Second

func PluginServices(home, roleName string) []ServiceStatus {
	return PluginServicesContext(context.Background(), home, roleName)
}

func PluginServicesContext(ctx context.Context, home, roleName string) []ServiceStatus {
	entries := plugin.CollectStatusEntries(home, roleName)
	type slot struct {
		idx int
		svc ServiceStatus
		ok  bool
	}
	done := make(chan slot, len(entries))
	pending := 0
	for i, e := range entries {
		if e.Contrib.Status == nil {
			continue
		}
		pending++
		go func(idx int, id string, c vkglyph.StatusContribution) {
			g, tone := c.Status()
			if g == "" {
				done <- slot{idx: idx}
				return
			}
			name := c.BrandGlyph
			if name == "" && c.Info != nil {
				name = c.Info()
			}
			done <- slot{
				idx: idx,
				svc: ServiceStatus{ID: id, Name: name, Severity: tone, Glyph: g},
				ok:  true,
			}
		}(i, e.PluginID, e.Contrib)
	}
	if pending == 0 {
		return nil
	}

	budget, cancel := context.WithTimeout(ctx, statusContribBudget)
	defer cancel()

	got := make([]ServiceStatus, len(entries))
	have := make([]bool, len(entries))
	for pending > 0 {
		select {
		case s := <-done:
			pending--
			if s.ok {
				got[s.idx] = s.svc
				have[s.idx] = true
			}
		case <-budget.Done():
			pending = 0
			for i, e := range entries {
				if e.Contrib.Status != nil && !have[i] {
					log.Debugf("deck: status contribution %s timed out", e.PluginID)
				}
			}
		}
	}

	out := make([]ServiceStatus, 0, len(entries))
	for i := range entries {
		if have[i] {
			out = append(out, got[i])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func RoleServices() []ServiceStatus {
	chips := role.StatusChips()
	if len(chips) == 0 {
		return nil
	}
	out := make([]ServiceStatus, 0, len(chips))
	for _, c := range chips {
		out = append(out, ServiceStatus{
			ID:       fmt.Sprintf("role-status-%d", c.Index),
			Name:     c.Glyph,
			Detail:   c.Text,
			Severity: vkglyph.SeverityPositive,
		})
	}
	return out
}

// adaptStatus applies mino policy to the raw chips: drops user-hidden ones,
// swaps known tool names for their logos, and renders the identity segment.
// Glyph/color resolution from severity is viewkit's job now.
func adaptStatus(info StatusInfo) vkdeck.StatusInfo {
	out := vkdeck.StatusInfo{Identity: identity(info)}
	for _, s := range info.Services {
		if statusBarHidden(s) {
			continue
		}
		s.Name, s.Detail = serviceChip(s.Name, s.Detail)
		s.Glyph = glyph.Lead(s.Glyph)
		out.Services = append(out.Services, s)
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

func serviceLabel(name string) string {
	if logo := glyph.ForTool(name); logo != "" {
		return logo
	}
	return name
}

func serviceChip(name, detail string) (string, string) {
	if logo := glyph.ForTool(name); logo != "" {
		if detail != "" {
			return glyph.Lead(logo) + detail, ""
		}
		return logo, ""
	}
	return name, detail
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
