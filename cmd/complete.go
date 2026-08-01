package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/plugin"
)

type completer func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

func completeNames(load func() []string) completer {
	return completeArg(0, load)
}

func completeArg(pos int, load func() []string) completer {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) != pos {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return load(), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeFlagValues(load func() []string) completer {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return load(), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeDirs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func completeBackupFiles(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return []string{"enc"}, cobra.ShellCompDirectiveFilterFileExt
}

func bindFlagCompletion(c *cobra.Command, flag string, fn completer) {
	if err := c.RegisterFlagCompletionFunc(flag, fn); err != nil {
		panic(err)
	}
}

func completeFlightNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(visibleFlightNames)(cmd, args, toComplete)
}

func completeQueryNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(visibleQueryNames)(cmd, args, toComplete)
}

func completeFilterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(visibleFilterNames)(cmd, args, toComplete)
}

func completeFormatterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(visibleFormatterNames)(cmd, args, toComplete)
}

func completeSignalNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeFlagValues(suggest.Signals)(cmd, args, toComplete)
}

func completeActionSignals(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(suggest.ActionSignals)(cmd, args, toComplete)
}

func completeListKinds(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func() []string { return listKinds })(cmd, args, toComplete)
}

func completeRoleNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeFlagValues(func() []string { return suggest.RoleNames(shared) })(cmd, args, toComplete)
}

func completeDirectives(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(suggest.Directives)(cmd, args, toComplete)
}

func completeInstalledPlugins(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(suggest.InstalledPluginIDs)(cmd, args, toComplete)
}

func completePluginIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(suggest.PluginIDs)(cmd, args, toComplete)
}

func completeDetailSignals(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeFlagValues(suggest.DetailSignals)(cmd, args, toComplete)
}

func completeActionRun(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return suggest.ActionSignals(), cobra.ShellCompDirectiveNoFileComp
	case 1:
		return suggest.ActionNames(args[0]), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func completeContextSwitch(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	ctxs := plugin.ListContexts(cmd.Context())
	seen := map[string]bool{}
	var out []string
	for _, c := range ctxs {
		switch len(args) {
		case 0:
			if !seen[c.Tool] {
				seen[c.Tool] = true
				out = append(out, c.Tool)
			}
		case 1:
			if c.Tool == args[0] {
				out = append(out, c.Name)
			}
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeParamAssignments(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	signal, _ := cmd.Flags().GetString("signal")
	if signal == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return suggest.ParamAssignments(signal), cobra.ShellCompDirectiveNoSpace | cobra.ShellCompDirectiveNoFileComp
}

func bindRootCompletions(root *cobra.Command) {
	bindFlagCompletion(root, "output", completeFlagValues(suggest.OutputFormats))
	bindFlagCompletion(root, "reconcile", completeFlagValues(suggest.ReconcilePolicies))
	bindFlagCompletion(root, "role", completeRoleNames)
	bindFlagCompletion(root, "home", completeDirs)
	bindFlagCompletion(root, "dir", completeDirs)
	bindRootFlagRules(root)
}

func bindRootFlagRules(root *cobra.Command) {
	root.MarkFlagsMutuallyExclusive("no-cache", "refresh")
}
