package gdocs

import (
	"context"

	drive "google.golang.org/api/drive/v3"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/gdrive"
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
	svc, err := auth.GoogleService(ctx, s.auth, "docs", []string{drive.DriveMetadataReadonlyScope}, drive.NewService)
	if err != nil {
		return nil, err
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
		items = append(items, docItem(f))
	}

	return []signals.Section{{
		Signal: "docs",
		Title:  "Recent Docs",
		Items:  items,
	}}, nil
}

func docItem(f *drive.File) signals.Item {
	item := gdrive.FileToItem(f)
	item.Kind = "doc"
	item.Meta = map[string]string{}
	return item
}
