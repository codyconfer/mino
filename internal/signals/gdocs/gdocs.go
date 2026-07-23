package gdocs

import (
	"context"
	"time"

	drive "google.golang.org/api/drive/v3"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultRecent = 10

type gdocsSignal struct {
	recent int
	auth   auth.GoogleAuth
}

func New(recent int, ga auth.GoogleAuth) signals.Signal {
	return &gdocsSignal{recent: recent, auth: ga}
}

func (s *gdocsSignal) Name() string { return "docs" }

func (s *gdocsSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	opt, err := auth.GoogleClientOption(ctx, s.auth, drive.DriveMetadataReadonlyScope)
	if err != nil {
		return nil, err
	}
	svc, err := drive.NewService(ctx, opt)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "docs: creating service")
	}

	recent := s.recent
	if recent <= 0 {
		recent = defaultRecent
	}

	res, err := svc.Files.List().
		Q("mimeType='application/vnd.google-apps.document' and trashed=false").
		OrderBy("modifiedTime desc").
		PageSize(int64(recent)).
		Fields("files(id,name,modifiedTime,webViewLink,owners(displayName))").
		Context(ctx).
		Do()
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "docs: listing recent documents")
	}

	items := make([]signals.Item, 0, len(res.Files))
	for _, f := range res.Files {
		items = append(items, fileToItem(f))
	}

	return []signals.Section{{
		Signal: "docs",
		Title:  "Recent Docs",
		Items:  items,
	}}, nil
}

func fileToItem(f *drive.File) signals.Item {
	item := signals.Item{
		Kind:  "doc",
		Title: f.Name,
		Body:  "",
		URL:   f.WebViewLink,
		Meta:  map[string]string{},
	}

	if len(f.Owners) > 0 && f.Owners[0] != nil {
		item.Subtitle = f.Owners[0].DisplayName
	}

	if f.ModifiedTime != "" {
		if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
			item.Timestamp = t
		}
	}

	return item
}
