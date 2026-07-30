package config

import (
	"bytes"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

func SetValues(homeOverride string, values map[string]any) (string, error) {
	home, name, raw, format, err := readConfigFileNamed(homeOverride)
	if err != nil {
		return "", err
	}
	if name == "" {
		name = "config.yaml"
		format = "yaml"
	}

	var out []byte
	if format == "json" {
		out, err = setJSONValues(raw, values)
	} else {
		out, err = setYAMLValues(raw, values)
	}
	if err != nil {
		return "", err
	}

	path, err := sconfig.WriteItem(home, name, out)
	if err != nil {
		return "", errs.Wrap(errs.KindConfig, err, "write config file")
	}
	return path, nil
}

func setJSONValues(raw []byte, values map[string]any) ([]byte, error) {
	doc := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, errs.Wrap(errs.KindConfig, err, "parse config file")
		}
	}
	for _, key := range sortedKeys(values) {
		setDotted(doc, key, values[key])
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "marshal config file")
	}
	return append(out, '\n'), nil
}

func setYAMLValues(raw []byte, values map[string]any) ([]byte, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, errs.Wrap(errs.KindConfig, err, "parse config file")
		}
	}
	root, err := documentMapping(&doc)
	if err != nil {
		return nil, err
	}
	for _, key := range sortedKeys(values) {
		if err := setNodePath(root, strings.Split(key, "."), values[key]); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return nil, errs.Wrap(errs.KindConfig, err, "marshal config file")
	}
	if err := enc.Close(); err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "marshal config file")
	}
	return buf.Bytes(), nil
}

func documentMapping(doc *yaml.Node) (*yaml.Node, error) {
	doc.Kind = yaml.DocumentNode
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{newMappingNode()}
		return doc.Content[0], nil
	}
	root := doc.Content[0]
	switch {
	case root.Kind == yaml.MappingNode:
		return root, nil
	case root.Kind == 0, root.Tag == "!!null":
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
		root.Value = ""
		root.Style = 0
		return root, nil
	}
	return nil, errs.New(errs.KindConfig, "config file is not a mapping of settings").
		WithHint("the top level of the config file must be `key: value` lines")
}

func setNodePath(m *yaml.Node, parts []string, val any) error {
	if len(parts) > 1 {
		child, err := mappingChild(m, parts[0])
		if err != nil {
			return err
		}
		return setNodePath(child, parts[1:], val)
	}
	var next yaml.Node
	if err := next.Encode(val); err != nil {
		return errs.Wrapf(errs.KindConfig, err, "encoding value for %q", parts[0])
	}
	if cur := mappingValue(m, parts[0]); cur != nil {
		next.HeadComment = cur.HeadComment
		next.LineComment = cur.LineComment
		next.FootComment = cur.FootComment
		*cur = next
		return nil
	}
	m.Content = append(m.Content, newKeyNode(parts[0]), &next)
	return nil
}

func mappingChild(m *yaml.Node, key string) (*yaml.Node, error) {
	cur := mappingValue(m, key)
	if cur == nil {
		child := newMappingNode()
		m.Content = append(m.Content, newKeyNode(key), child)
		return child, nil
	}
	switch {
	case cur.Kind == yaml.MappingNode:
		return cur, nil
	case cur.Kind == 0, cur.Tag == "!!null":
		cur.Kind = yaml.MappingNode
		cur.Tag = "!!map"
		cur.Value = ""
		cur.Style = 0
		return cur, nil
	}
	return nil, errs.Newf(errs.KindConfig, "cannot add settings under %q: it already holds a single value", key).
		WithHint("replace `%s: %s` with a nested block, or edit the config file by hand", key, cur.Value)
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func newKeyNode(key string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
}

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
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
