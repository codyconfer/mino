package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	internalapp "github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/onboard"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/plugin"
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

var bootstrapOnce sync.Once

func Run(opts Options) error {
	bootstrapOnce.Do(internalapp.Bootstrap)
	err := run(opts)
	if err != nil {
		fmt.Fprint(os.Stderr, errs.Render(err))
	}
	return err
}

func run(opts Options) (err error) {
	if opts.CLI == nil {
		return ErrNoCLI
	}
	applyBuildPolicy(opts)
	internalapp.SetDefaultsFS(opts.Defaults)

	args := opts.Args
	if args == nil {
		args = os.Args[1:]
	}

	registerPlugins(opts.RegisterPlugins)
	if reportDiagnosticsFor(args) {
		ReportPluginDiagnostics(os.Stderr)
	}

	ctx := context.Background()
	if opts.BeforeRun != nil {
		if err := opts.BeforeRun(ctx); err != nil {
			return err
		}
	}

	defer func() {
		if opts.AfterRun != nil {
			opts.AfterRun(ctx, err)
		}
	}()

	err = opts.CLI(ctx, args)
	return err
}

// EnvPluginDiagnostics suppresses the stderr plugin-problem report when set to
// one of 0/off/false/quiet/none. Diagnostics stay available in `mino plugins
// list`.
const EnvPluginDiagnostics = "MINO_PLUGIN_DIAGNOSTICS"

// registerPlugins is defence in depth: every SDK entry point now skips a bad
// contribution with a diagnostic instead of panicking, so nothing here should
// ever fire. A panic raised by a plugin's own registration code still cannot be
// contained per-contribution from out here (the hook is a single callback), so
// the diagnostic names the plugin that was mid-registration and says plainly
// that the rest of registration was dropped. Hosts that want per-plugin
// containment should wrap each plugin in plugin.Guarded.
func registerPlugins(fn func()) {
	if fn == nil {
		return
	}
	_, seenBefore := plugin.RegistrationCheckpoint()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		who, seenAfter := plugin.RegistrationCheckpoint()
		where := "before it registered any contribution"
		if seenAfter == seenBefore {
			who = ""
		} else {
			where = fmt.Sprintf("while registering %q", who)
		}
		plugin.NoteDiagnostic(who, "", "", fmt.Sprintf(
			"plugin registration panicked %s: %v; registration was truncated, so this plugin's remaining contributions and every later plugin's contributions were skipped",
			where, r))
	}()
	fn()
}

// reportDiagnosticsFor keeps the stderr report out of output a user cannot act
// on: shell completion (where stray stderr breaks the UX) and explicit opt-out.
func reportDiagnosticsFor(args []string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvPluginDiagnostics))) {
	case "0", "off", "false", "quiet", "none":
		return false
	}
	for _, a := range args {
		if strings.HasPrefix(a, "__complete") || a == "completion" {
			return false
		}
	}
	return true
}

func ReportPluginDiagnostics(w io.Writer) {
	if w == nil {
		return
	}
	for _, d := range plugin.Diagnostics() {
		fmt.Fprintf(w, "mino: plugin problem: %s\n", d)
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
