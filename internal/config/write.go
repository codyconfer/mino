package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/errs"
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
		out, err = setYAMLValues(name, raw, values)
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
	if out, ordered, err := setJSONValuesInOrder(raw, values); err != nil || ordered {
		return out, err
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

func setJSONValuesInOrder(raw []byte, values map[string]any) ([]byte, bool, error) {
	root := newMappingNode()
	if len(bytes.TrimSpace(raw)) > 0 {
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, false, nil
		}
		if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
			return nil, false, nil
		}
		root = doc.Content[0]
	}
	for _, key := range sortedKeys(values) {
		if err := setNodePath(root, strings.Split(key, "."), values[key]); err != nil {
			return nil, false, err
		}
	}
	var buf bytes.Buffer
	if err := writeJSONNode(&buf, root, 0); err != nil {
		return nil, false, nil
	}
	buf.WriteByte('\n')
	return buf.Bytes(), true, nil
}

func writeJSONNode(buf *bytes.Buffer, n *yaml.Node, depth int) error {
	switch n.Kind {
	case yaml.MappingNode:
		return writeJSONMapping(buf, n, depth)
	case yaml.SequenceNode:
		return writeJSONSequence(buf, n, depth)
	case yaml.AliasNode:
		if n.Alias == nil {
			return errs.New(errs.KindConfig, "unresolved alias in the config file")
		}
		return writeJSONNode(buf, n.Alias, depth)
	case yaml.ScalarNode:
		var v any
		if err := n.Decode(&v); err != nil {
			return errs.Wrap(errs.KindConfig, err, "marshal config file")
		}
		encoded, err := json.Marshal(v)
		if err != nil {
			return errs.Wrap(errs.KindConfig, err, "marshal config file")
		}
		buf.Write(encoded)
		return nil
	}
	return errs.New(errs.KindConfig, "the config file holds a value JSON cannot represent")
}

func writeJSONMapping(buf *bytes.Buffer, n *yaml.Node, depth int) error {
	if len(n.Content) < 2 {
		buf.WriteString("{}")
		return nil
	}
	buf.WriteString("{\n")
	for i := 0; i+1 < len(n.Content); i += 2 {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString(jsonIndent(depth + 1))
		key, err := json.Marshal(n.Content[i].Value)
		if err != nil {
			return errs.Wrap(errs.KindConfig, err, "marshal config file")
		}
		buf.Write(key)
		buf.WriteString(": ")
		if err := writeJSONNode(buf, n.Content[i+1], depth+1); err != nil {
			return err
		}
	}
	buf.WriteString("\n" + jsonIndent(depth) + "}")
	return nil
}

func writeJSONSequence(buf *bytes.Buffer, n *yaml.Node, depth int) error {
	if len(n.Content) == 0 {
		buf.WriteString("[]")
		return nil
	}
	buf.WriteString("[\n")
	for i, item := range n.Content {
		if i > 0 {
			buf.WriteString(",\n")
		}
		buf.WriteString(jsonIndent(depth + 1))
		if err := writeJSONNode(buf, item, depth+1); err != nil {
			return err
		}
	}
	buf.WriteString("\n" + jsonIndent(depth) + "]")
	return nil
}

func jsonIndent(depth int) string { return strings.Repeat("  ", depth) }

func setYAMLValues(name string, raw []byte, values map[string]any) ([]byte, error) {
	docs, err := yamlDocuments(raw)
	if err != nil {
		return nil, err
	}
	if len(docs) > 1 {
		return nil, errs.Newf(errs.KindConfig, "%s holds %d YAML documents", name, len(docs)).
			WithHint("mino reads only the first one, so editing settings here would delete the other %d; "+
				"merge them into a single document (drop the `---` separators) or edit the file by hand", len(docs)-1)
	}
	if len(docs) == 0 {
		return newYAMLDocument(raw, values)
	}
	doc := docs[0]
	root, err := documentMapping(doc)
	if err != nil {
		return nil, err
	}
	for _, key := range sortedKeys(values) {
		if err := setNodePath(root, strings.Split(key, "."), values[key]); err != nil {
			return nil, err
		}
	}
	untagMergeKeys(doc)
	unflowUnquotableScalars(doc)
	return encodeYAMLDocument(doc)
}

func yamlDocuments(raw []byte) ([]*yaml.Node, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var out []*yaml.Node
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, errs.Wrap(errs.KindConfig, err, "parse config file")
		}
		out = append(out, &doc)
	}
}

func newYAMLDocument(raw []byte, values map[string]any) ([]byte, error) {
	root := newMappingNode()
	for _, key := range sortedKeys(values) {
		if err := setNodePath(root, strings.Split(key, "."), values[key]); err != nil {
			return nil, err
		}
	}
	body, err := encodeYAMLDocument(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return body, nil
	}
	out := append([]byte(nil), raw...)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return append(out, body...), nil
}

func encodeYAMLDocument(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return nil, errs.Wrap(errs.KindConfig, err, "marshal config file")
	}
	if err := enc.Close(); err != nil {
		return nil, errs.Wrap(errs.KindConfig, err, "marshal config file")
	}
	return buf.Bytes(), nil
}

func untagMergeKeys(n *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Tag == "!!merge" {
				n.Content[i].Tag = ""
			}
		}
	}
	for _, c := range n.Content {
		untagMergeKeys(c)
	}
}

const (
	leadingFlowIndicators  = "#,[]{}&*!|>'\"%@`"
	anywhereFlowIndicators = ",?[]{}:\n"
)

func unquotableInFlow(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode || n.Style != 0 || n.Value == "" {
		return false
	}
	if strings.ContainsAny(n.Value[:1], leadingFlowIndicators) {
		return true
	}
	if strings.HasPrefix(n.Value, "- ") || strings.HasPrefix(n.Value, "---") {
		return true
	}
	return strings.ContainsAny(n.Value, anywhereFlowIndicators) || strings.Contains(n.Value, " #")
}

func flowSubtreeNeedsBlock(n *yaml.Node) bool {
	if unquotableInFlow(n) {
		return true
	}
	for _, c := range n.Content {
		if flowSubtreeNeedsBlock(c) {
			return true
		}
	}
	return false
}

func clearFlowStyle(n *yaml.Node) {
	n.Style &= ^yaml.FlowStyle
	for _, c := range n.Content {
		clearFlowStyle(c)
	}
}

func unflowUnquotableScalars(n *yaml.Node) {
	collection := n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode
	if collection && n.Style&yaml.FlowStyle != 0 {
		if flowSubtreeNeedsBlock(n) {
			clearFlowStyle(n)
		}
		return
	}
	for _, c := range n.Content {
		unflowUnquotableScalars(c)
	}
}

func documentMapping(doc *yaml.Node) (*yaml.Node, error) {
	doc.Kind = yaml.DocumentNode
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{newMappingNode()}
		return doc.Content[0], nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.AliasNode && root.Alias != nil {
		flattenAlias(root)
	}
	switch {
	case root.Kind == yaml.MappingNode:
		return root, nil
	case root.Kind == 0, root.Tag == "!!null":
		makeMappingNode(root)
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
	cur := mappingValue(m, parts[0])
	if cur == nil {
		m.Content = append(m.Content, newKeyNode(parts[0]), &next)
		return nil
	}
	if err := checkLeafReplacement(parts[0], cur, &next); err != nil {
		return err
	}
	next.HeadComment = cur.HeadComment
	next.LineComment = cur.LineComment
	next.FootComment = cur.FootComment
	*cur = next
	return nil
}

func checkLeafReplacement(key string, cur, next *yaml.Node) error {
	if next.Kind != yaml.ScalarNode {
		return nil
	}
	target := cur
	if target.Kind == yaml.AliasNode && target.Alias != nil {
		target = target.Alias
	}
	if target.Kind != yaml.MappingNode {
		return nil
	}
	return errs.Newf(errs.KindConfig, "cannot set %q to a single value: it holds a block of settings", key).
		WithHint("replacing the `%s:` block with one value would drop every setting and comment inside it; "+
			"set one of the keys under `%s:` instead, or edit the config file by hand", key, key)
}

func mappingChild(m *yaml.Node, key string) (*yaml.Node, error) {
	cur := mappingValue(m, key)
	if cur == nil {
		child := newMappingNode()
		m.Content = append(m.Content, newKeyNode(key), child)
		return child, nil
	}
	anchor := ""
	if cur.Kind == yaml.AliasNode {
		anchor = cur.Value
		if cur.Alias == nil {
			return nil, errs.Newf(errs.KindConfig, "cannot add settings under %q: it is the alias `*%s` and mino cannot find that anchor", key, anchor).
				WithHint("give %q a nested block of its own, or edit the config file by hand", key)
		}
		flattenAlias(cur)
	}
	switch {
	case cur.Kind == yaml.MappingNode:
		return cur, nil
	case cur.Kind == 0, cur.Tag == "!!null":
		makeMappingNode(cur)
		return cur, nil
	}
	if anchor != "" {
		return nil, errs.Newf(errs.KindConfig, "cannot add settings under %q: it is the alias `*%s`, and the anchor `&%s` holds the single value %q",
			key, anchor, anchor, cur.Value).
			WithHint("replace `%s: *%s` with a nested block, or edit the config file by hand", key, anchor)
	}
	return nil, errs.Newf(errs.KindConfig, "cannot add settings under %q: it already holds a single value", key).
		WithHint("replace `%s: %s` with a nested block, or edit the config file by hand", key, cur.Value)
}

func flattenAlias(n *yaml.Node) {
	resolved := cloneYAMLNode(n.Alias)
	resolved.HeadComment = n.HeadComment
	resolved.LineComment = n.LineComment
	resolved.FootComment = n.FootComment
	*n = *resolved
}

func cloneYAMLNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	out := *n
	out.Anchor = ""
	out.Alias = nil
	out.Content = nil
	for _, c := range n.Content {
		out.Content = append(out.Content, cloneYAMLNode(c))
	}
	return &out
}

func makeMappingNode(n *yaml.Node) {
	n.Kind = yaml.MappingNode
	n.Tag = "!!map"
	n.Value = ""
	n.Style = 0
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
