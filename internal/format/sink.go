package format

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/munin/internal/errs"
)

func Deliver(out, status io.Writer, text string, copyFn func(string) error, outPath string) error {
	text = strings.TrimRight(text, "\n") + "\n"

	delivered := false

	if outPath != "" {
		dir := filepath.Dir(outPath)
		if !sconfig.IsDir(dir) {
			return errs.Newf(errs.KindUsage, "output directory %q does not exist", dir).
				WithHint("create it first: mkdir -p %s", dir)
		}
		written, err := sconfig.WriteItem(dir, filepath.Base(outPath), []byte(text))
		if err != nil {
			return errs.Wrapf(errs.KindUsage, err, "writing report to %q", outPath)
		}
		report(status, "wrote %s", written)
		delivered = true
	}

	if copyFn != nil {
		if err := copyFn(text); err != nil {
			return errs.Wrap(errs.KindUsage, err, "copying report to clipboard")
		}
		report(status, "copied %d bytes", len(text))
		delivered = true
	}

	if delivered || out == nil {
		return nil
	}
	if _, err := io.WriteString(out, text); err != nil {
		return errs.Wrap(errs.KindUsage, err, "writing report")
	}
	return nil
}

func report(status io.Writer, format string, args ...any) {
	if status == nil {
		return
	}
	fmt.Fprintf(status, format+"\n", args...)
}
