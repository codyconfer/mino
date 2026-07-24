package config

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

func SetValues(homeOverride string, values map[string]any) (string, error) {
	home, raw, format, err := ReadConfigFile(homeOverride)
	if err != nil {
		return "", err
	}

	doc := map[string]any{}
	if len(raw) > 0 {
		if format == "json" {
			err = json.Unmarshal(raw, &doc)
		} else {
			err = yaml.Unmarshal(raw, &doc)
		}
		if err != nil {
			return "", errs.Wrap(errs.KindConfig, err, "parse config file")
		}
	}

	for key, val := range values {
		setDotted(doc, key, val)
	}

	var out []byte
	if format == "json" {
		out, err = json.MarshalIndent(doc, "", "  ")
	} else {
		format = "yaml"
		out, err = yaml.Marshal(doc)
	}
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "marshal config file")
	}

	path, err := sconfig.WriteConfigFile(home, out, format)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "write config file")
	}
	return path, nil
}

func setDotted(doc map[string]any, dotted string, val any) {
	parts := strings.Split(dotted, ".")
	cur := doc
	for _, p := range parts[:len(parts)-1] {
		child, ok := cur[p].(map[string]any)
		if !ok {
			child = map[string]any{}
			cur[p] = child
		}
		cur = child
	}
	cur[parts[len(parts)-1]] = val
}
