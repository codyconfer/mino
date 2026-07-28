package loginflow

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/render"
)

func RunCLI(ctx context.Context, a *app.App, p Provider, in io.Reader, out, errOut io.Writer) error {
	if p.Authed(a) {
		fmt.Fprintln(out, notify.Render(notify.AlreadyAuthed(p.Label)))
		return nil
	}

	creds := map[string]string{}
	if missing := p.Missing(a); len(missing) > 0 && term.IsTerminal(os.Stdin.Fd()) {
		reader := bufio.NewReader(in)
		fmt.Fprintf(out, "%s needs OAuth client credentials — enter them to continue.\n", p.Label)
		for _, f := range missing {
			fmt.Fprintf(out, "  %s: ", f.Label)
			val, err := readCredential(reader, f.Secret)
			if err != nil {
				return err
			}
			if val == "" {
				return errs.Newf(errs.KindUsage, "%s is required", f.Label)
			}
			creds[f.Key] = val
		}
		if err := PersistCredentials(a, creds); err != nil {
			return err
		}
	}

	if err := p.Login(ctx, a, creds, errOut); err != nil {
		return err
	}
	fmt.Fprintln(out, render.Success(p.Label+" authorized — token cached."))
	return nil
}

func readCredential(reader *bufio.Reader, secret bool) (string, error) {
	if secret {
		b, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Fprintln(os.Stdout)
		if err != nil {
			return "", errs.Wrap(errs.KindUsage, err, "reading credential input")
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", errs.Wrap(errs.KindUsage, err, "reading credential input")
	}
	return strings.TrimSpace(line), nil
}

func ResolveOrErr(name string) (Provider, error) {
	p, ok := Resolve(name)
	if !ok {
		return Provider{}, errs.Newf(errs.KindUsage, "unsupported login target %q", name).
			WithHint("supported: %s", strings.Join(Names(), ", "))
	}
	return p, nil
}
