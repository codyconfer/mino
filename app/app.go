package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	internalapp "github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/plugin"
)

var ErrNoCLI = errors.New("app: Options.CLI is required (wire cmd.Root().ExecuteContext)")

type Options struct {
	Args []string

	Version string

	EmailDomain string

	AllOrNothingAuth bool

	// Deprecated: use AllOrNothingAuth; the two are OR-ed together.
	EnforceAuth bool

	Defaults fs.FS

	CLI func(ctx context.Context, args []string) error

	RegisterPlugins func()

	BeforeRun func(context.Context) error

	AfterRun func(context.Context, error)
}

func Run(opts Options) (err error) {
	if opts.CLI == nil {
		return ErrNoCLI
	}
	applyBuildPolicy(opts)
	internalapp.SetDefaultsFS(opts.Defaults)
	registerPlugins(opts.RegisterPlugins)
	ReportPluginDiagnostics(os.Stderr)

	ctx := context.Background()
	if opts.BeforeRun != nil {
		if err := opts.BeforeRun(ctx); err != nil {
			return err
		}
	}

	args := opts.Args
	if args == nil {
		args = os.Args[1:]
	}

	defer func() {
		if opts.AfterRun != nil {
			opts.AfterRun(ctx, err)
		}
	}()

	err = opts.CLI(ctx, args)
	return err
}

func registerPlugins(fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			plugin.NoteDiagnostic("", "", "", fmt.Sprintf("plugin registration panicked: %v", r))
		}
	}()
	fn()
}

func ReportPluginDiagnostics(w io.Writer) {
	if w == nil {
		return
	}
	for _, d := range plugin.Diagnostics() {
		fmt.Fprintf(w, "munin: plugin problem: %s\n", d)
	}
}

func applyBuildPolicy(opts Options) {
	if opts.EmailDomain != "" {
		onboard.RequiredEmailDomain = opts.EmailDomain
	}
	if opts.AllOrNothingAuth || opts.EnforceAuth {
		onboard.AllOrNothingAuth = "true"
	}
}
