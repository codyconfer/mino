package cmd

import "github.com/spf13/cobra"

func newSlackCmd() *cobra.Command {
	parent := &cobra.Command{Use: "slack", Short: "Slack channel activity"}

	var ff filterFlags
	var channel string
	query := &cobra.Command{
		Use:   "query",
		Short: "Fetch a Slack channel now, with optional filters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]string{}
			if channel != "" {
				params["channel"] = channel
			}
			return runSignal(cmd, "slack", params, &ff)
		},
	}
	query.Flags().StringVar(&channel, "channel", "", "channel to read (name or ID)")
	ff.bind(query)
	parent.AddCommand(query)
	return parent
}
