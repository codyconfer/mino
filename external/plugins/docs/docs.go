package docs

import (
	"context"

	driveapi "google.golang.org/api/drive/v3"

	"github.com/codyconfer/munin/external/plugins/drive"
	"github.com/codyconfer/munin/external/plugins/internal/errx"
	"github.com/codyconfer/munin/external/plugins/internal/googleauth"
	"github.com/codyconfer/munin/plugin"
)

const defaultRecent = 10

type gdocsSignal struct {
	recent int
	auth   googleauth.Auth
}

func New(recent int, ga googleauth.Auth) plugin.Query {
	return &gdocsSignal{recent: recent, auth: ga}
}

func (s *gdocsSignal) Name() string { return "docs" }

func (s *gdocsSignal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	svc, err := googleauth.Service(ctx, s.auth, "docs", []string{driveapi.DriveMetadataReadonlyScope}, driveapi.NewService)
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
		return nil, errx.Wrap(err, "docs: listing recent documents")
	}

	items := make([]plugin.Item, 0, len(res.Files))
	for _, f := range res.Files {
		items = append(items, docItem(f))
	}

	return []plugin.Section{{
		Signal: "docs",
		Title:  "Recent Docs",
		Items:  items,
	}}, nil
}

func docItem(f *driveapi.File) plugin.Item {
	item := drive.FileToItem(f)
	item.Kind = "doc"
	item.Meta = map[string]string{}
	return item
}
