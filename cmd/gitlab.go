package cmd

import "github.com/spf13/cobra"

func newGitlabCmd() *cobra.Command {
	return sourceCmd("gitlab", "GitLab activity (merge requests, pipelines, issues)")
}
