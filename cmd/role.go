package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
)

func dashRole(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newRoleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "role",
		Short: "Show the active role and defined roles",
		Long: "Roles scope what munin shows. Activate one with --role <name>, the\n" +
			"MUNIN_ROLE env var, or `role:` in config.yaml. A role names the flights,\n" +
			"queries, and filters that appear in lists and the TUI; with no active\n" +
			"role, everything is listed. Role YAML may also set contexts:, hooks:\n" +
			"(enter/exit bash or PowerShell), and status: blocks (glyph + command;\n" +
			"truncated stdout appears in the status bar while the role is active).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			a := access()

			role := a.Role
			if role == "" {
				role = "(none)"
			}
			fmt.Fprintf(out, "active role:  %s\n", role)

			names := shared.Directives.RoleNames()
			if len(names) == 0 {
				fmt.Fprintln(out, "\nno roles defined (add a <name>.yaml in the config dir)")
				return nil
			}
			fmt.Fprintln(out, "\ndefined roles:")
			for _, n := range names {
				rd := shared.Directives.Roles[n]
				marker := "  "
				if n == a.Role {
					marker = theme.Cur().Can.Render(glyph.StatusOK()) + " "
				}
				fmt.Fprintf(out, "%s%-12s home=%s flights=[%s] queries=[%s] formatters=[%s]\n",
					marker, n, dashRole(rd.Home),
					strings.Join(rd.Flights, ", "),
					strings.Join(rd.Queries, ", "),
					strings.Join(rd.Formatters, ", "))
			}
			return nil
		},
	}
}
