package slack

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/codyconfer/mino/plugin"
)

//go:embed all:seeds
var seedFS embed.FS

func seedFiles() []plugin.FileSeed {
	var out []plugin.FileSeed
	err := fs.WalkDir(seedFS, "seeds", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		body, readErr := seedFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out = append(out, plugin.FileSeed{
			RelPath: strings.TrimPrefix(path, "seeds/"),
			Content: body,
		})
		return nil
	})
	if err != nil {
		plugin.NoteDiagnostic(PluginID, plugin.KindSignal, SignalName,
			"could not read embedded query seeds: "+err.Error())
		return nil
	}
	return out
}
