package views

import (
	"errors"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func directiveSplit(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func directiveFindingLine(th theme.Theme, f Finding) string {
	var mark string
	switch {
	case f.OK:
		mark = th.Can.Render(glyph.Check())
	case f.Warn:
		mark = th.Dim.Render(glyph.Warn())
	default:
		mark = th.Cant.Render(glyph.Cross())
	}
	return mark + " " + th.Val.Render(f.Name)
}

func directiveNoFileNote(name string) string {
	return "no file on disk for " + name + ".\n\n" +
		"It may exist only in DuckDB (the source of truth); use\n" +
		"`mino export directives` to write files first."
}

func (kit *Kit) directiveMultiDocNote(rel string) string {
	n := kit.d.App.Dirs().DocCount(rel)
	if n <= 1 {
		return ""
	}
	return rel + " holds " + strconv.Itoa(n) + " directives; edit it by hand to keep the others intact."
}

func (kit *Kit) saveDirective(kind config.DirectiveType, rel, name string, doc any) (string, bool, error) {
	if note := kit.directiveMultiDocNote(rel); note != "" {
		return "", false, errs.New(errs.KindUsage, note)
	}
	path, stored, err := config.SaveDirective(kit.d.App.Mgr, kit.d.App.Cfg.Home, rel, kind, name, doc)
	if err != nil {
		return "", false, err
	}
	if !stored {
		return "wrote " + path + "\n\n" +
			"the config store is unavailable, so this file takes effect after\n" +
			"reconcile: run `mino import directives` or restart mino.", false, nil
	}
	return "wrote " + path + "\nimported the directives collection into DuckDB.", true, nil
}

func (kit *Kit) removeDirective(kind config.DirectiveType, name string) ([]string, string) {
	rel := kit.d.App.Dirs().Source(kind, name)
	if rel == "" {
		return nil, directiveNoFileNote(name)
	}
	if note := kit.directiveMultiDocNote(rel); note != "" {
		return nil, "did not remove " + name + ": " + note
	}
	removed, err := config.RemoveDirective(kit.d.App.Cfg.Home, rel)
	if err != nil {
		return nil, err.Error()
	}
	if len(removed) == 0 {
		return nil, directiveNoFileNote(name)
	}
	return removed, ""
}

func (kit *Kit) deleteDirective(kind config.DirectiveType, name string) (string, error) {
	removed, note := kit.removeDirective(kind, name)
	if note != "" {
		return "", errors.New(note)
	}
	summary := "removed:\n  " + strings.Join(removed, "\n  ")
	stored, err := config.SyncDirectives(kit.d.App.Mgr, kit.d.App.Cfg.Home)
	switch {
	case err != nil:
		return summary + "\n\nthe store still holds it: " + err.Error(), nil
	case !stored:
		return summary + "\n\nthe config store is unavailable, so this takes effect after\n" +
			"reconcile: run `mino import directives` or restart mino.", nil
	}
	if err := kit.d.App.RefreshDirectives(config.ReconcileIgnore); err != nil {
		return summary + "\nremoved from DuckDB.\n\nreload failed: " + err.Error(), nil
	}
	return summary + "\nremoved from DuckDB; the change is live in this session.", nil
}
