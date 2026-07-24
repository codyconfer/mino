package cmd

import "github.com/spf13/cobra"

func newGmailCmd() *cobra.Command {
	return sourceCmd("gmail", "Matching Gmail messages")
}
