package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/signals"
	gmailsrc "github.com/codyconfer/munin/internal/signals/gmail"
)

func newGmailCmd() *cobra.Command {
	return sourceCmd("gmail", "Matching Gmail messages")
}

func buildGmail(params map[string]string) (signals.Signal, error) {
	query := paramStr(params, "query", shared.cfg.Gmail.Query)
	max := paramInt(params, "max", shared.cfg.Gmail.Max)
	return gmailsrc.New(query, max, googleAuth()), nil
}
