package audit

import (
	"fmt"
	"io"
	"time"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
)

func PrintRecent(w io.Writer, s *Store, limit int) error {
	runs, err := s.RecentEntries(limit)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Fprintln(w, "no recorded runs (audit trail is empty or disabled)")
		return nil
	}
	for _, r := range runs {
		fmt.Fprintf(w, "%-6d %s  %-6s %-18s role=%-8s %s\n",
			r.ID, r.Started.Format("2006-01-02 15:04"), r.Kind, r.Name, dash(r.Role), status(r))
	}
	return nil
}

func PrintEntry(w io.Writer, s *Store, id int64) error {
	run, ok, err := s.Entry(id)
	if err != nil {
		return err
	}
	if !ok {
		return errs.Newf(errs.KindUsage, "no run with id %d", id)
	}
	printRow(w, run)

	if run.Kind == "flight" {
		children, err := s.Children(id)
		if err != nil {
			return err
		}
		for _, ch := range children {
			fmt.Fprintln(w)
			printRow(w, ch)
			printItems(w, s, ch.ID)
		}
		return nil
	}
	printItems(w, s, id)
	return nil
}

func printRow(w io.Writer, r AuditRow) {
	dur := ""
	if !r.Finished.IsZero() {
		dur = " in " + r.Finished.Sub(r.Started).Round(time.Millisecond).String()
	}
	fmt.Fprintf(w, "#%d  %s  %s %q  (role=%s)  %s%s\n",
		r.ID, r.Started.Format("2006-01-02 15:04:05"), r.Kind, r.Name, dash(r.Role), status(r), dur)
}

func printItems(w io.Writer, s *Store, runID int64) {
	items, err := s.Items(runID)
	if err != nil || len(items) == 0 {
		return
	}
	for _, it := range items {
		when := ""
		if !it.Ts.IsZero() {
			when = "  " + it.Ts.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "    %s %s  %s%s\n", glyph.Bullet(), it.Title, dash(it.Subtitle), when)
		if it.URL != "" {
			fmt.Fprintf(w, "      %s\n", it.URL)
		}
	}
}

func status(r AuditRow) string {
	if r.Error != "" {
		return "error: " + r.Error
	}
	return fmt.Sprintf("%d items", r.ItemCount)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
