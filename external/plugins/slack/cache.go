package slack

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	chanCacheTTL  = 10 * time.Minute
	chanCacheKeep = 256
	userCacheTTL  = 30 * time.Minute
	userCacheKeep = 1024
	whoamiTTL     = 12 * time.Hour
	whoamiKeep    = 16
	userBatch     = 30
)

type ttlCache[V any] struct {
	mu   sync.Mutex
	m    map[string]cacheEntry[V]
	ttl  time.Duration
	keep int
}

type cacheEntry[V any] struct {
	val V
	at  time.Time
}

func newTTLCache[V any](ttl time.Duration, keep int) *ttlCache[V] {
	return &ttlCache[V]{ttl: ttl, keep: keep}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	var zero V
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Since(e.at) > c.ttl {
		return zero, false
	}
	return e.val, true
}

func (c *ttlCache[V]) put(key string, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > c.keep {
		c.m = map[string]cacheEntry[V]{}
	}
	c.m[key] = cacheEntry[V]{val: val, at: time.Now()}
}

func (c *ttlCache[V]) reset() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}

type chanRef struct {
	id   string
	name string
}

type whoami struct {
	userID string
	host   string
}

var (
	chanCache   = newTTLCache[chanRef](chanCacheTTL, chanCacheKeep)
	userCache   = newTTLCache[string](userCacheTTL, userCacheKeep)
	whoamiCache = newTTLCache[whoami](whoamiTTL, whoamiKeep)
)

func cacheKey(token, want string) string { return token + "\x00" + want }

func resetChannelCache() { chanCache.reset() }

func resetUserCache() { userCache.reset() }

func resetWhoamiCache() { whoamiCache.reset() }

func resetCaches() {
	resetChannelCache()
	resetUserCache()
	resetWhoamiCache()
}

func (s *slackSignal) me(ctx context.Context, api *slackapi.Client) (whoami, error) {
	key := cacheKey(s.token, "whoami")
	if hit, ok := whoamiCache.get(key); ok {
		return hit, nil
	}
	resp, err := api.AuthTestContext(ctx)
	if err != nil {
		return whoami{}, errx.Wrap(err, "slack: identifying the token (auth.test)").
			WithHint("set `plugins.slack.workspace` to build permalinks without this call")
	}
	who := whoami{userID: resp.UserID, host: hostFromURL(resp.URL)}
	whoamiCache.put(key, who)
	return who, nil
}

func hostFromURL(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	if i := strings.IndexAny(v, "/?#"); i >= 0 {
		v = v[:i]
	}
	return v
}

func (s *slackSignal) resolveUsers(ctx context.Context, api *slackapi.Client, ids []string) map[string]string {
	out := make(map[string]string, len(ids))
	if !s.resolveNames {
		return out
	}
	var missing []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if name, ok := userCache.get(cacheKey(s.token, id)); ok {
			out[id] = name
			continue
		}
		missing = append(missing, id)
	}

	for len(missing) > 0 {
		batch := missing
		if len(batch) > userBatch {
			batch = batch[:userBatch]
		}
		missing = missing[len(batch):]

		users, err := api.GetUsersInfoContext(ctx, batch...)
		if err != nil {
			noteNameFailure(err)
			return out
		}
		if users == nil {
			return out
		}
		for _, u := range *users {
			name := displayName(u)
			if name == "" {
				continue
			}
			userCache.put(cacheKey(s.token, u.ID), name)
			out[u.ID] = name
		}
	}
	return out
}

var nameFailureOnce sync.Once

func noteNameFailure(err error) {
	nameFailureOnce.Do(func() {
		plugin.NoteDiagnostic(PluginID, plugin.KindSignal, SignalName,
			"could not resolve Slack display names ("+err.Error()+
				"); items show raw user ids. Re-run `mino login slack` to grant users:read")
	})
}

func displayName(u slackapi.User) string {
	if v := strings.TrimSpace(u.Profile.DisplayName); v != "" {
		return v
	}
	if v := strings.TrimSpace(u.RealName); v != "" {
		return v
	}
	return strings.TrimSpace(u.Name)
}

func (s *slackSignal) resolveChannel(ctx context.Context, api *slackapi.Client, channel string) (id, name string, err error) {
	if isChannelID(channel) {
		if hit, ok := chanCache.get(cacheKey(s.token, channel)); ok {
			return hit.id, hit.name, nil
		}
		return channel, "", nil
	}

	want := strings.TrimPrefix(channel, "#")
	if hit, ok := chanCache.get(cacheKey(s.token, want)); ok {
		return hit.id, hit.name, nil
	}

	cursor := ""
	giveUp := ""
	for page := 0; ; page++ {
		if page >= maxListPages {
			giveUp = fmt.Sprintf("stopped after %d pages", maxListPages)
			break
		}
		channels, next, listErr := api.GetConversationsContext(ctx, &slackapi.GetConversationsParameters{
			Types:  []string{"public_channel", "private_channel"},
			Cursor: cursor,
			Limit:  listPageSize,
		})
		if listErr != nil {
			return "", "", errx.Wrapf(listErr, "slack: listing conversations (page %d)", page+1).
				WithHint("retry in a moment, or set the channel by ID (C.../G.../D...) to skip the channel walk")
		}
		for _, ch := range channels {
			cacheChannel(s.token, ch.Name, ch.ID, ch.Name)
			cacheChannel(s.token, ch.ID, ch.ID, ch.Name)
		}
		for _, ch := range channels {
			if ch.Name == want {
				return ch.ID, ch.Name, nil
			}
		}
		if next == "" {
			break
		}
		if next == cursor {
			giveUp = "conversations.list returned the same pagination cursor twice"
			break
		}
		cursor = next
	}

	if giveUp != "" {
		return "", "", errx.Newf("slack: gave up resolving channel %q: %s", channel, giveUp).
			WithHint("use the channel ID (C.../G.../D...) instead of its name to skip the channel walk")
	}
	return "", "", errx.Newf("slack: channel %q not found", channel).
		WithHint("check the channel name, or use its ID (C.../G.../D...)")
}

func cacheChannel(token, key, id, name string) {
	if key == "" {
		return
	}
	chanCache.put(cacheKey(token, key), chanRef{id: id, name: name})
}
