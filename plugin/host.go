package plugin

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type Credential struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	Expiry       time.Time
}

type CredentialStore interface {
	Get(ctx context.Context, service string) (Credential, bool, error)
	Put(ctx context.Context, service string, c Credential) error
	Delete(ctx context.Context, service string) error
}

type Host interface {
	Home() string
	Role() string
	Settings(namespace string) map[string]any
	Credentials() CredentialStore
}

func HostOf(bc BuildContext) (Host, bool) {
	h, ok := bc.(Host)
	if !ok || h == nil {
		return nil, false
	}
	return h, true
}

func SettingsOf(bc BuildContext, namespace string) map[string]any {
	h, ok := HostOf(bc)
	if !ok {
		return nil
	}
	return h.Settings(namespace)
}

func CredentialsOf(bc BuildContext) CredentialStore {
	h, ok := HostOf(bc)
	if !ok {
		return nil
	}
	return h.Credentials()
}

func Setting(settings map[string]any, key, def string) string {
	v, ok := settings[key]
	if !ok {
		return def
	}
	s := settingString(v)
	if s == "" {
		return def
	}
	return s
}

func SettingInt(settings map[string]any, key string, def int) int {
	v, ok := settings[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(settingString(v))); err == nil {
		return n
	}
	return def
}

func SettingBool(settings map[string]any, key string, def bool) bool {
	v, ok := settings[key]
	if !ok {
		return def
	}
	if b, isBool := v.(bool); isBool {
		return b
	}
	switch strings.ToLower(strings.TrimSpace(settingString(v))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func SettingDuration(settings map[string]any, key string, def time.Duration) time.Duration {
	raw := Setting(settings, key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	return d
}

func SettingList(settings map[string]any, key string) []string {
	v, ok := settings[key]
	if !ok {
		return nil
	}
	switch list := v.(type) {
	case []string:
		return trimmedList(list)
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			out = append(out, settingString(item))
		}
		return trimmedList(out)
	}
	return trimmedList(strings.Split(settingString(v), ","))
}

func trimmedList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func settingString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(s)
	case bool:
		return strconv.FormatBool(s)
	case int:
		return strconv.Itoa(s)
	case int64:
		return strconv.FormatInt(s, 10)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	}
	return ""
}
