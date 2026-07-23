package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "role",
		Short: "Show the active role and defined roles",
		Long: "Roles scope what munin shows. Activate one with --role <name>, the\n" +
			"MUNIN_ROLE env var, or `role:` in config.yaml. A role names the flights,\n" +
			"queries, and filters that appear in lists and the TUI; with no active\n" +
			"role, everything is listed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			a := access()

			role := a.Role
			if role == "" {
				role = "(none)"
			}
			fmt.Fprintf(out, "active role:  %s\n", role)

			names := shared.directives.RoleNames()
			if len(names) == 0 {
				fmt.Fprintln(out, "\nno roles defined (add files under roles/)")
				return nil
			}
			fmt.Fprintln(out, "\ndefined roles:")
			for _, n := range names {
				rd := shared.directives.Roles[n]
				marker := "  "
				if n == a.Role {
					marker = "* "
				}
				fmt.Fprintf(out, "%s%-12s flights=[%s] queries=[%s] filters=[%s]\n",
					marker, n,
					strings.Join(rd.Flights, ", "),
					strings.Join(rd.Queries, ", "),
					strings.Join(rd.Filters, ", "))
			}
			return nil
		},
	}
}
