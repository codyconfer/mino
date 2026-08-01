package backup

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/plugin"
)

type fakeDestination struct {
	uploadedName string
	uploadedData []byte
}

func (f *fakeDestination) Name() string { return "faketest" }

func (f *fakeDestination) Upload(_ context.Context, name string, data []byte, _ string) (plugin.Item, error) {
	f.uploadedName = name
	f.uploadedData = append([]byte(nil), data...)
	return plugin.Item{Title: name}, nil
}

func (f *fakeDestination) Prune(context.Context, string, int) ([]string, error) { return nil, nil }

func TestUploadToOpensFreshTokenStore(t *testing.T) {
	ctx := context.Background()
	sink := &fakeDestination{}
	var creds plugin.CredentialStore
	probeOK := true
	probeErr := errors.New("open func never ran")
	plugin.RegisterBackupDestination("test.fake", "faketest", func(h plugin.Host) (plugin.BackupDestination, error) {
		creds = h.Credentials()
		_, probeOK, probeErr = creds.Get(ctx, "faketest")
		return sink, nil
	})
	t.Cleanup(plugin.ResetBackupDestinations)

	var buf bytes.Buffer
	cfg := &config.Config{Home: t.TempDir()}
	if err := uploadTo(ctx, &buf, cfg, "faketest", "mino-backup-x.tar.enc", []byte("sealed"), "keyring", 0); err != nil {
		t.Fatalf("uploadTo: %v", err)
	}
	if probeErr != nil || probeOK {
		t.Fatalf("probe Get = ok %v err %v, want a clean miss from a fresh store", probeOK, probeErr)
	}
	if sink.uploadedName != "mino-backup-x.tar.enc" || string(sink.uploadedData) != "sealed" {
		t.Fatalf("Upload got name %q data %q", sink.uploadedName, sink.uploadedData)
	}
	if creds == nil {
		t.Fatal("destination never received a credential store")
	}
	if _, _, err := creds.Get(ctx, "faketest"); err == nil {
		t.Fatal("Get after uploadTo should error: the store must be closed once the upload finishes")
	}
}

func TestUploadToUnknownDestination(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{Home: t.TempDir()}
	if err := uploadTo(context.Background(), &buf, cfg, "nope", "n", []byte("x"), "keyring", 0); err == nil {
		t.Fatal("expected error for unknown destination")
	}
}
