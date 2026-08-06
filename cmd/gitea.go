package cmd

import "github.com/spf13/cobra"

func newGiteaCmd() *cobra.Command {
	c := sourceCmd("gitea", "Gitea/Forgejo activity (PRs, issues, review requests)")
	c.Aliases = []string{"forgejo"}
	return c
}
