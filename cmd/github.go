package cmd

import "github.com/spf13/cobra"

func newGithubCmd() *cobra.Command {
	return sourceCmd("github", "GitHub activity (PRs, review requests)")
}
