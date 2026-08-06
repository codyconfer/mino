package slack

import (
	"strconv"
	"strings"
	"time"
)

const titleCap = 120

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func parseSlackTS(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	secPart := ts
	if i := strings.IndexByte(ts, '.'); i >= 0 {
		secPart = ts[:i]
	}
	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func isChannelID(v string) bool {
	if v == "" {
		return false
	}
	switch v[0] {
	case 'C', 'G', 'D':
		return true
	default:
		return false
	}
}

func unfurl(text string, names map[string]string) string {
	if !strings.ContainsRune(text, '<') {
		return text
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(text, '<')
		if i < 0 {
			b.WriteString(text)
			break
		}
		rest := text[i+1:]
		j := strings.IndexByte(rest, '>')
		if j < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:i])
		b.WriteString(unfurlToken(rest[:j], names))
		text = rest[j+1:]
	}
	return b.String()
}

func unfurlToken(tok string, names map[string]string) string {
	if tok == "" {
		return ""
	}
	body, label, hasLabel := strings.Cut(tok, "|")
	if body == "" {
		return label
	}
	switch body[0] {
	case '@':
		id := body[1:]
		if name, ok := names[id]; ok && name != "" {
			return "@" + name
		}
		if hasLabel && label != "" {
			return "@" + strings.TrimPrefix(label, "@")
		}
		return "@" + id
	case '#':
		if hasLabel && label != "" {
			return "#" + strings.TrimPrefix(label, "#")
		}
		return "#" + body[1:]
	case '!':
		if hasLabel && label != "" {
			return "@" + strings.TrimPrefix(label, "@")
		}
		return "@" + body[1:]
	default:
		if hasLabel && label != "" {
			return label
		}
		return body
	}
}

func mentionIDs(text string, into map[string]bool) {
	for {
		i := strings.Index(text, "<@")
		if i < 0 {
			return
		}
		rest := text[i+2:]
		j := strings.IndexByte(rest, '>')
		if j < 0 {
			return
		}
		id, _, _ := strings.Cut(rest[:j], "|")
		if id != "" {
			into[id] = true
		}
		text = rest[j+1:]
	}
}
