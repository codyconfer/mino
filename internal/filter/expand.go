package filter

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/codyconfer/munin/internal/errs"
)

var ExternalKeywords func(name string) (map[string]string, bool)

var (
	bracedAliasRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	qualifierRe   = regexp.MustCompile(`(?i)\b(created|updated|closed):\(([^)]+)\)`)
	relativeRe    = regexp.MustCompile(`(?i)^(\d+)\s+(day|days|week|weeks|hour|hours)\s+ago$`)
)

func TemplateContext(filters []Filter) (map[string]string, error) {
	ctx := make(map[string]string)
	put := func(kind, filterName, key, val string) error {
		key = strings.TrimSpace(key)
		if key == "" {
			return errs.Newf(errs.KindConfig, "filter %q has empty %s key", filterName, kind)
		}
		if prev, ok := ctx[key]; ok && prev != val {
			return errs.Newf(errs.KindConfig, "filter %q redefines %s %q (was %q)", filterName, kind, key, prev)
		}
		ctx[key] = val
		return nil
	}
	for _, f := range filters {
		for k, v := range f.Aliases {
			if err := put("alias", f.Name, k, v); err != nil {
				return nil, err
			}
		}
		for k, v := range f.Keywords {
			if err := put("keyword", f.Name, k, v); err != nil {
				return nil, err
			}
		}
		if ExternalKeywords != nil {
			if m, ok := ExternalKeywords(f.Name); ok {
				for k, v := range m {
					if err := put("keyword", f.Name, k, v); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return ctx, nil
}

func ExpandParams(params map[string]string, filters []Filter) (map[string]string, error) {
	if len(params) == 0 {
		return params, nil
	}
	ctx, err := TemplateContext(filters)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		expanded, err := Expand(v, ctx)
		if err != nil {
			return nil, errs.Wrapf(errs.KindConfig, err, "expanding param %q", k)
		}
		out[k] = expanded
	}
	return out, nil
}

func Expand(s string, ctx map[string]string) (string, error) {
	if s == "" {
		return s, nil
	}
	out, err := expandRelativeQualifiers(s, time.Now)
	if err != nil {
		return "", err
	}
	out = expandBracedAliases(out, ctx)
	if !strings.Contains(out, "{{") {
		return out, nil
	}
	return executeTemplate(out, ctx, time.Now)
}

func expandBracedAliases(s string, ctx map[string]string) string {
	if len(ctx) == 0 || !strings.Contains(s, "{") {
		return s
	}
	return bracedAliasRe.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := ctx[name]; ok {
			return v
		}
		return m
	})
}

func expandRelativeQualifiers(s string, now func() time.Time) (string, error) {
	if !strings.Contains(s, ":(") {
		return s, nil
	}
	var firstErr error
	out := qualifierRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := qualifierRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return m
		}
		qual := strings.ToLower(parts[1])
		phrase := strings.TrimSpace(parts[2])
		if !relativeRe.MatchString(phrase) {
			return m
		}
		day, err := relativeDay(phrase, now())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return qual + ":>=" + day.UTC().Format("2006-01-02")
	})
	return out, firstErr
}

func executeTemplate(s string, ctx map[string]string, now func() time.Time) (string, error) {
	tmpl, err := template.New("query").Option("missingkey=error").Funcs(templateFuncs(now)).Parse(s)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "parse query template")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "execute query template")
	}
	return buf.String(), nil
}

func templateFuncs(now func() time.Time) template.FuncMap {
	qual := func(name string) func(string) (string, error) {
		return func(phrase string) (string, error) {
			day, err := relativeDay(phrase, now())
			if err != nil {
				return "", err
			}
			return name + ":>=" + day.UTC().Format("2006-01-02"), nil
		}
	}
	return template.FuncMap{
		"created": qual("created"),
		"updated": qual("updated"),
		"closed":  qual("closed"),
		"ago": func(phrase string) (string, error) {
			day, err := relativeDay(phrase, now())
			if err != nil {
				return "", err
			}
			return day.UTC().Format("2006-01-02"), nil
		},
	}
}

func relativeDay(phrase string, now time.Time) (time.Time, error) {
	phrase = strings.TrimSpace(phrase)
	m := relativeRe.FindStringSubmatch(phrase)
	if len(m) != 3 {
		return time.Time{}, fmt.Errorf("unsupported relative time %q (want e.g. \"3 days ago\")", phrase)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 {
		return time.Time{}, fmt.Errorf("unsupported relative time %q", phrase)
	}
	unit := strings.ToLower(m[2])
	switch unit {
	case "day", "days":
		return now.AddDate(0, 0, -n), nil
	case "week", "weeks":
		return now.AddDate(0, 0, -7*n), nil
	case "hour", "hours":
		return now.Add(-time.Duration(n) * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported relative time unit %q", unit)
	}
}
