package slack

import (
	"strings"
)

const archivesSegment = "/archives/"

type msgRef struct {
	ChannelID string
	TS        string
	ThreadTS  string
}

func (r msgRef) ok() bool { return r.ChannelID != "" && r.TS != "" }

func (r msgRef) root() string {
	if r.ThreadTS != "" {
		return r.ThreadTS
	}
	return r.TS
}

func permalink(host string, r msgRef) string {
	if host == "" || !r.ok() {
		return ""
	}
	url := "https://" + host + archivesSegment + r.ChannelID + "/" + tsPathSegment(r.TS)
	if r.ThreadTS != "" && r.ThreadTS != r.TS {
		url += "?thread_ts=" + r.ThreadTS + "&cid=" + r.ChannelID
	}
	return url
}

func tsPathSegment(ts string) string {
	return "p" + strings.Replace(ts, ".", "", 1)
}

func parsePermalink(raw string) (msgRef, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return msgRef{}, false
	}
	query := ""
	if i := strings.IndexAny(v, "?#"); i >= 0 {
		query, v = v[i+1:], v[:i]
	}
	i := strings.Index(v, archivesSegment)
	if i < 0 {
		return msgRef{}, false
	}
	parts := strings.Split(strings.Trim(v[i+len(archivesSegment):], "/"), "/")
	if len(parts) < 2 {
		return msgRef{}, false
	}
	if !isChannelID(parts[0]) {
		return msgRef{}, false
	}
	ts, ok := tsFromPathSegment(parts[1])
	if !ok {
		return msgRef{}, false
	}
	return msgRef{ChannelID: parts[0], TS: ts, ThreadTS: threadTSFromQuery(query)}, true
}

func threadTSFromQuery(query string) string {
	for _, pair := range strings.Split(query, "&") {
		name, value, found := strings.Cut(pair, "=")
		if !found || name != "thread_ts" {
			continue
		}
		if allDigitsExceptOneDot(value) {
			return value
		}
	}
	return ""
}

func tsFromPathSegment(seg string) (string, bool) {
	v := strings.TrimPrefix(seg, "p")
	if v == "" {
		return "", false
	}
	if strings.Contains(v, ".") {
		if !allDigitsExceptOneDot(v) {
			return "", false
		}
		return v, true
	}
	if !allDigits(v) {
		return "", false
	}
	if len(v) <= 6 {
		return "", false
	}
	return v[:len(v)-6] + "." + v[len(v)-6:], true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func allDigitsExceptOneDot(s string) bool {
	dot := strings.IndexByte(s, '.')
	if dot <= 0 || dot == len(s)-1 {
		return false
	}
	return allDigits(s[:dot]) && allDigits(s[dot+1:])
}
