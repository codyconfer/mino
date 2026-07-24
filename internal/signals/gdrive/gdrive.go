package gdrive

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	drive "google.golang.org/api/drive/v3"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
)

const folderMime = "application/vnd.google-apps.folder"

type gdriveSignal struct {
	auth    auth.GoogleAuth
	folders []string
	recent  int
}

func New(ga auth.GoogleAuth, folders []string, recent int) signals.Signal {
	return &gdriveSignal{auth: ga, folders: folders, recent: recent}
}

func (s *gdriveSignal) Name() string { return "drive" }

func (s *gdriveSignal) Fetch(ctx context.Context) ([]signals.Section, error) {
	svc, err := newService(ctx, s.auth)
	if err != nil {
		return nil, err
	}
	recent := s.recent
	if recent <= 0 {
		recent = 20
	}

	q := "trashed = false"
	if len(s.folders) > 0 {
		ids, err := resolveFolders(ctx, svc, s.folders)
		if err != nil {
			return nil, err
		}
		var clauses []string
		for _, id := range ids {
			clauses = append(clauses, fmt.Sprintf("'%s' in parents", id))
		}
		q = "(" + strings.Join(clauses, " or ") + ") and trashed = false"
	}

	res, err := svc.Files.List().
		Q(q).
		OrderBy("modifiedTime desc").
		PageSize(int64(recent)).
		Fields("files(id,name,mimeType,modifiedTime,webViewLink,owners(displayName))").
		Context(ctx).Do()
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "drive: listing files")
	}

	sec := signals.Section{Signal: "drive", Title: "Drive"}
	for _, f := range res.Files {
		sec.Items = append(sec.Items, FileToItem(f))
	}
	return []signals.Section{sec}, nil
}

func CreateFile(ctx context.Context, ga auth.GoogleAuth, dirRef, name, content, mime string) (signals.Item, error) {
	svc, err := newService(ctx, ga)
	if err != nil {
		return signals.Item{}, err
	}
	dirID, dirName, err := resolveFolder(ctx, svc, dirRef)
	if err != nil {
		return signals.Item{}, err
	}
	f := &drive.File{Name: name, Parents: []string{dirID}}
	if mime != "" {
		f.MimeType = mime
	}
	call := svc.Files.Create(f).
		Fields("id,name,mimeType,modifiedTime,webViewLink,owners(displayName)").
		Context(ctx)
	if content != "" {
		call = call.Media(strings.NewReader(content))
	}
	created, err := call.Do()
	if err != nil {
		return signals.Item{}, errs.Wrapf(errs.KindSignal, err, "drive: creating file in %q", dirName)
	}
	item := FileToItem(created)
	if item.Subtitle == "" {
		item.Subtitle = dirName
	}
	return item, nil
}

func resolveFolder(ctx context.Context, svc *drive.Service, ref string) (id, name string, err error) {
	res, err := svc.Files.List().
		Q(fmt.Sprintf("mimeType = '%s' and trashed = false", folderMime)).
		PageSize(1000).
		Fields("files(id,name)").
		Context(ctx).Do()
	if err != nil {
		return "", "", errs.Wrap(errs.KindSignal, err, "drive: listing folders")
	}
	var names []string
	for _, f := range res.Files {
		names = append(names, f.Name)
		if f.Id == ref || strings.EqualFold(f.Name, ref) {
			return f.Id, f.Name, nil
		}
	}
	return "", "", errs.Newf(errs.KindSignal, "drive: folder %q not found", ref).
		WithHint("available folders: %s", strings.Join(names, ", "))
}

func resolveFolders(ctx context.Context, svc *drive.Service, refs []string) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		id, _, err := resolveFolder(ctx, svc, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func newService(ctx context.Context, ga auth.GoogleAuth) (*drive.Service, error) {
	return auth.GoogleService(ctx, ga, "drive", drive.DriveScope, drive.NewService)
}

func UploadAppData(ctx context.Context, ga auth.GoogleAuth, name string, content []byte, mime string) (signals.Item, error) {
	svc, err := auth.GoogleService(ctx, ga, "drive", drive.DriveAppdataScope, drive.NewService)
	if err != nil {
		return signals.Item{}, err
	}
	f := &drive.File{Name: name, Parents: []string{"appDataFolder"}}
	if mime != "" {
		f.MimeType = mime
	}
	created, err := svc.Files.Create(f).
		Media(bytes.NewReader(content)).
		Fields("id,name,mimeType,modifiedTime").
		Context(ctx).Do()
	if err != nil {
		return signals.Item{}, errs.Wrap(errs.KindSignal, err, "drive: uploading to app data folder")
	}
	item := FileToItem(created)
	item.Subtitle = "Drive app data (private)"
	return item, nil
}

func PruneAppData(ctx context.Context, ga auth.GoogleAuth, namePrefix string, keep int) ([]string, error) {
	if keep <= 0 {
		return nil, nil
	}
	svc, err := auth.GoogleService(ctx, ga, "drive", drive.DriveAppdataScope, drive.NewService)
	if err != nil {
		return nil, err
	}
	res, err := svc.Files.List().
		Spaces("appDataFolder").
		OrderBy("createdTime desc").
		PageSize(1000).
		Fields("files(id,name)").
		Context(ctx).Do()
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "drive: listing app data files")
	}
	var matched []*drive.File
	for _, f := range res.Files {
		if strings.HasPrefix(f.Name, namePrefix) {
			matched = append(matched, f)
		}
	}
	var deleted []string
	for i := keep; i < len(matched); i++ {
		if err := svc.Files.Delete(matched[i].Id).Context(ctx).Do(); err != nil {
			return deleted, errs.Wrapf(errs.KindSignal, err, "drive: deleting app data file %q", matched[i].Name)
		}
		deleted = append(deleted, matched[i].Name)
	}
	return deleted, nil
}

func FileToItem(f *drive.File) signals.Item {
	var ts time.Time
	if f.ModifiedTime != "" {
		ts, _ = time.Parse(time.RFC3339, f.ModifiedTime)
	}
	subtitle := f.MimeType
	if len(f.Owners) > 0 && f.Owners[0].DisplayName != "" {
		subtitle = f.Owners[0].DisplayName
	}
	meta := map[string]string{}
	if f.MimeType != "" {
		meta["mime"] = f.MimeType
	}
	if f.Id != "" {
		meta["id"] = f.Id
	}
	return signals.Item{
		Kind:      "file",
		Title:     f.Name,
		Subtitle:  subtitle,
		URL:       f.WebViewLink,
		Timestamp: ts,
		Meta:      meta,
	}
}
