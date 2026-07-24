package cmd

import "github.com/spf13/cobra"

func newDocsCmd() *cobra.Command {
	return sourceCmd("docs", "Recently modified Google Docs")
}
