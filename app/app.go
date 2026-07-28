package app

import (
	"context"
	"errors"
	"io/fs"
	"os"

	internalapp "github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/onboard"
)

var ErrNoCLI = errors.New("app: Options.CLI is required (wire cmd.Root().ExecuteContext)")

type Options struct {
	Args []string

	Version string

	EmailDomain string

	AllOrNothingAuth bool

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
	if opts.RegisterPlugins != nil {
		opts.RegisterPlugins()
	}

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

func applyBuildPolicy(opts Options) {
	if opts.EmailDomain != "" {
		onboard.RequiredEmailDomain = opts.EmailDomain
	}
	if opts.AllOrNothingAuth {
		onboard.AllOrNothingAuth = "true"
	}
}
