package github

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	orgScopeHint    = "the read:org scope is required; run `gh auth refresh -s read:org` or re-run `mino login github`"
	teamCacheNS     = "github:team"
	teamCacheTTL    = 24 * time.Hour
	teamMemberPages = 20
)

type Cache interface {
	Get(ctx context.Context, namespace, key string) (string, bool)
	Put(ctx context.Context, namespace, key, value string, expiry time.Time)
}

type RosterCache = Cache

type Roster struct {
	slug    string
	members map[string]bool
}

func (r *Roster) Configured() bool { return r != nil && r.slug != "" }

func (r *Roster) Has(login string) bool {
	if r == nil || login == "" {
		return false
	}
	return r.members[strings.ToLower(login)]
}

func newRoster(slug string, logins []string) *Roster {
	members := make(map[string]bool, len(logins))
	for _, l := range logins {
		if l = strings.TrimSpace(l); l != "" {
			members[strings.ToLower(l)] = true
		}
	}
	return &Roster{slug: slug, members: members}
}

func ParseTeamRef(raw string) (string, string, error) {
	ref := strings.Trim(strings.TrimSpace(raw), "/")
	if i := strings.Index(ref, "github.com"); i >= 0 {
		ref = strings.Trim(ref[i+len("github.com"):], "/")
		ref = strings.TrimPrefix(ref, "orgs/")
		ref = strings.Replace(ref, "/teams/", "/", 1)
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errs.Newf(errs.KindConfig, "github: cannot parse team reference %q", raw).
			WithHint("use `owner/team-slug` (e.g. acme/platform)")
	}
	return parts[0], parts[1], nil
}

func ResolveTeam(ctx context.Context, backend Backend, cache RosterCache, ref string, pol CachePolicy) (*Roster, error) {
	org, slug, err := ParseTeamRef(ref)
	if err != nil {
		return nil, err
	}
	key := org + "/" + slug
	if cache != nil && pol.reads() {
		if raw, ok := cache.Get(ctx, teamCacheNS, key); ok {
			return newRoster(key, strings.Split(raw, "\n")), nil
		}
	}
	logins, err := fetchTeamMembers(ctx, backend, org, slug)
	if err != nil {
		return nil, err
	}
	if cache != nil && pol.writes() {
		cache.Put(ctx, teamCacheNS, key, strings.Join(logins, "\n"), time.Now().Add(teamCacheTTL))
	}
	log.Debugf("github: resolved %d member(s) of team %s", len(logins), key)
	return newRoster(key, logins), nil
}

func fetchTeamMembers(ctx context.Context, backend Backend, org, slug string) ([]string, error) {
	var logins []string
	cursor := ""
	for range teamMemberPages {
		vars := map[string]any{"org": org, "team": slug}
		if cursor != "" {
			vars["after"] = cursor
		}
		raw, err := backend.GraphQL(ctx, teamMembersQuery, vars)
		if err != nil {
			return nil, err
		}
		var resp graphQLResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, errs.Wrap(errs.KindSignal, err, "github: decoding team members response")
		}
		if err := resp.errHint(orgScopeHint); err != nil {
			return nil, err
		}
		if resp.Data.Organization == nil || resp.Data.Organization.Team == nil {
			return nil, errs.Newf(errs.KindConfig, "github: team %s/%s not found", org, slug).
				WithHint("use `owner/team-slug` (e.g. acme/platform); the authenticated user must be an org member")
		}
		members := resp.Data.Organization.Team.Members
		for _, n := range members.Nodes {
			if n.Login != "" {
				logins = append(logins, n.Login)
			}
		}
		if !members.PageInfo.HasNextPage || members.PageInfo.EndCursor == "" {
			break
		}
		cursor = members.PageInfo.EndCursor
	}
	return logins, nil
}

const teamMembersQuery = `query($org:String!,$team:String!,$after:String){
  organization(login:$org){
    team(slug:$team){
      members(first:100,after:$after){
        pageInfo{hasNextPage endCursor}
        nodes{login}
      }
    }
  }
}`
