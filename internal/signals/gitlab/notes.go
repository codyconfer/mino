package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type noteWire struct {
	ID        int64    `json:"id"`
	Body      string   `json:"body"`
	System    bool     `json:"system"`
	CreatedAt string   `json:"created_at"`
	Author    userWire `json:"author"`
}

var botLoginSuffixes = []string{"-bot", "_bot", "[bot]"}

func (w noteWire) isBot() bool {
	if w.Author.Bot {
		return true
	}
	login := strings.ToLower(w.Author.Username)
	for _, suffix := range botLoginSuffixes {
		if strings.HasSuffix(login, suffix) {
			return true
		}
	}
	return false
}

func (s *Signal) fetchNotes(ctx context.Context, ref Ref) ([]noteWire, int, error) {
	p, err := s.backend.Get(ctx, ref.path("notes"), url.Values{
		"sort":     {"asc"},
		"order_by": {"created_at"},
		"per_page": {strconv.Itoa(notesPerPage)},
	})
	if err != nil {
		return nil, 0, err
	}
	var rows []noteWire
	if err := json.Unmarshal(p.Body, &rows); err != nil {
		return nil, 0, errs.Wrap(errs.KindSignal, err, "gitlab: decoding notes")
	}
	total := 0
	if p.HasTotal {
		total = p.Total
	}
	return rows, total, nil
}

// notesSection drops system notes. GitLab records "assigned to @x", "added 3 commits"
// and "changed target branch" as notes, so an unfiltered thread is mostly bookkeeping.
func notesSection(notes []noteWire, total int) (signals.DetailSection, bool) {
	human := make([]noteWire, 0, len(notes))
	for _, n := range notes {
		if n.System || strings.TrimSpace(n.Body) == "" {
			continue
		}
		human = append(human, n)
	}
	if len(human) == 0 {
		return signals.DetailSection{}, false
	}

	blocks := make([]string, 0, len(human))
	for _, n := range human {
		blocks = append(blocks, noteBlock(n))
	}
	return signals.DetailSection{
		Title: notesTitle(len(human), total),
		Body:  strings.Join(blocks, "\n\n---\n\n"),
	}, true
}

func noteBlock(n noteWire) string {
	who := atLogin(n.Author.Username)
	if who == "" {
		who = "@unknown"
	}
	if n.isBot() {
		who += " ·bot"
	}
	header := "### " + who
	if rel := relTime(n.CreatedAt); rel != "" {
		header += " · " + rel
	}
	return header + "\n\n" + strings.TrimSpace(n.Body)
}

func notesTitle(shown, total int) string {
	if total > shown {
		return fmt.Sprintf("comments (latest %d of %d)", shown, total)
	}
	return "comments"
}
