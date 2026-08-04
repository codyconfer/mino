package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/errs"
)

func dashRole(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newRoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Show the active role and defined roles",
		Long: "Roles scope what mino shows. `mino role use <name>` activates one for\n" +
			"good: it records the active role in mino's state store and runs the\n" +
			"previous role's exit hooks and the new role's enter hooks. It never\n" +
			"edits your config file — `role:` there is only the default applied\n" +
			"until a role is activated or cleared. `--role <name>` and the\n" +
			"MINO_ROLE env var scope a single invocation instead and run no hooks,\n" +
			"so they are safe in prompts, statuslines, and wrappers. A role names\n" +
			"the flights, queries, and filters that appear in lists and the TUI;\n" +
			"with no active role, everything is listed. Role YAML may also set\n" +
			"contexts:, hooks: (enter/exit bash or PowerShell), and status: blocks\n" +
			"(glyph + command; truncated stdout appears in the status bar while the\n" +
			"role is active).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			a := access()

			role := a.Role
			if role == "" {
				role = "(none)"
			}
			fmt.Fprintf(out, "active role:  %s\n", role)

			names := shared.Dirs().RoleNames()
			if len(names) == 0 {
				fmt.Fprintln(out, "\nno roles defined (add a <name>.yaml in the config dir)")
				return nil
			}
			fmt.Fprintln(out, "\ndefined roles:")
			sc := Scope()
			for _, n := range names {
				rd := shared.Dirs().Roles[n]
				marker := "  "
				if n == a.Role {
					marker = sc.Theme.Can.Render(sc.Glyphs.StatusOK()) + " "
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
	cmd.AddCommand(newRoleUseCmd(), newRoleClearCmd())
	return cmd
}

func newRoleUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Activate a role: record it as active and run its hooks",
		Long: "Records <name> as the active role, runs the previous role's exit hooks\n" +
			"then <name>'s enter hooks, applies its contexts, and refreshes its\n" +
			"status blocks. Your config file is left alone. Use --role or\n" +
			"MINO_ROLE to scope one invocation without recording anything and\n" +
			"without running hooks.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRoleNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRoleName(cmd, args); err != nil {
				return err
			}
			if err := shared.ActivateRole(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active role:  %s\n", args[0])
			return nil
		},
	}
}

func newRoleClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Deactivate the active role and run its exit hooks",
		Long: "Records that no role is active, runs the active role's exit hooks, and\n" +
			"drops its status blocks. Lists show everything again. The cleared\n" +
			"state outranks `role:` in config, so the default does not come back.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := shared.ActivateRole(""); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "active role:  (none)")
			return nil
		},
	}
}

func checkRoleName(cmd *cobra.Command, args []string) error {
	names := shared.Dirs().RoleNames()
	if len(names) == 0 {
		return errs.Newf(errs.KindUsage, "unknown role %q", args[0]).
			WithHint("no roles are defined; add a <name>.yaml with `type: role` in the config dir")
	}
	cmd.ValidArgs = names
	if err := cobra.OnlyValidArgs(cmd, args); err != nil {
		return errs.Newf(errs.KindUsage, "unknown role %q", args[0]).
			WithHint("defined roles: %s", strings.Join(names, ", "))
	}
	return nil
}
