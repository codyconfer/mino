package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
)

func newLoginCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "login <provider|signal>",
		Short: "Authenticate a signal provider via OAuth (guided)",
		Long: "Sign in to a signal provider. Accepts a provider (`github`, `google`,\n" +
			"`slack`) or any signal name — Google signals (`calendar`, `gmail`, `docs`,\n" +
			"`drive`, `tasks`) all resolve to the shared Google login. When run\n" +
			"interactively, munin prompts for any missing OAuth client credentials and\n" +
			"saves them to config before starting the browser/device flow. Tokens are\n" +
			"cached (encrypted) under <home> and used by the signal's direct API client.",
		Args:        cobra.ExactArgs(1),
		ValidArgs:   loginflow.Names(),
		Annotations: map[string]string{annoSkipOnboarding: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			p, ok := loginflow.Resolve(args[0])
			if !ok {
				return errs.Newf(errs.KindUsage, "unsupported login target %q", args[0]).
					WithHint("supported: %s", strings.Join(loginflow.Names(), ", "))
			}
			return runLogin(cmd, p)
		},
	}
	return c
}

func runLogin(cmd *cobra.Command, p loginflow.Provider) error {
	creds := map[string]string{}
	if missing := p.Missing(shared); len(missing) > 0 && stdinIsInteractive() {
		out := cmd.OutOrStdout()
		reader := bufio.NewReader(cmd.InOrStdin())
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
		if err := loginflow.PersistCredentials(shared, creds); err != nil {
			return err
		}
	}

	if err := p.Login(cmd.Context(), shared, creds, cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), render.Success(p.Label+" authorized — token cached."))
	return nil
}

func stdinIsInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd())
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
