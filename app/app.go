// Package app is the public distribution entrypoint (ADR-8 / M6).
//
// Team overlay binaries import this package, call [Run] with hooks and an
// optional go:embed defaults FS, and stamp domain / auth policy via [Options]
// or the traditional ldflags into internal/app/onboard.
//
// This package intentionally does not import munin/cmd so overlays and tests
// stay buildable while deck/term extraction (M5) churns. Callers supply [Options.CLI]
// (stock munin main and the overlay template both wire cmd.Root().ExecuteContext).
package app

import (
	"context"
	"errors"
	"io/fs"
	"os"

	internalapp "github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/onboard"
)

// ErrNoCLI is returned when Run is invoked without Options.CLI.
var ErrNoCLI = errors.New("app: Options.CLI is required (wire cmd.Root().ExecuteContext)")

// Options configures a munin (or overlay) binary.
type Options struct {
	// Args are CLI arguments without the program name. When nil, os.Args[1:] is used.
	Args []string

	// Version is informational for overlays (stamp via their own ldflags if desired).
	Version string

	// EmailDomain, when non-empty, restricts onboarding to that email domain
	// (equivalent to -X …onboard.RequiredEmailDomain=…).
	EmailDomain string

	// EnforceAuth hard-blocks unauthorized cli directives
	// (equivalent to -X …onboard.EnforceAuthorized=true).
	EnforceAuth bool

	// Defaults is an optional filesystem of seed config/directives for install.
	// Layout mirrors ~/.munin (config.yaml, queries/, filters/, flights/, roles/).
	// When set, Install/Nuke merge these seeds over the stock scaffold (overlay wins).
	Defaults fs.FS

	// CLI executes the root command after hooks. Required.
	// Stock munin and overlays typically pass cmd.Root().ExecuteContext (with SetArgs).
	CLI func(ctx context.Context, args []string) error

	// RegisterPlugins runs once before CLI. Overlays register compile-time plugins here.
	RegisterPlugins func()

	// BeforeRun runs after policy/plugin setup and before CLI.
	BeforeRun func(context.Context) error

	// AfterRun runs after CLI returns (err may be nil).
	AfterRun func(context.Context, error)
}

// Run applies distribution hooks and invokes Options.CLI.
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

// applyBuildPolicy sets onboard policy vars from Options. Safe to call from tests.
func applyBuildPolicy(opts Options) {
	if opts.EmailDomain != "" {
		onboard.RequiredEmailDomain = opts.EmailDomain
	}
	if opts.EnforceAuth {
		onboard.EnforceAuthorized = "true"
	}
}
