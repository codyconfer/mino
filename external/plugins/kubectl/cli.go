package kubectl

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/cmd"
)

func registerCommand() { cmd.RegisterCommand(newKubectlCmd) }

func newKubectlCmd() *cobra.Command {
	c := cmd.QueryCmd(SignalName, "Kubernetes cluster health", bindFlags)
	c.Aliases = []string{"k8s"}
	return c
}

func bindFlags(c *cobra.Command, params *map[string]string) {
	var (
		kubeContext   string
		namespace     string
		allNamespaces bool
		what          []string
		since         string
		limit         int
	)
	f := c.Flags()
	f.StringVar(&kubeContext, "context", "", "kube context to read (kubeconfig is never modified)")
	f.StringVarP(&namespace, "namespace", "n", "", "namespace to read")
	f.BoolVarP(&allNamespaces, "all-namespaces", "A", false, "read every namespace (default)")
	f.StringSliceVar(&what, "what", nil, "collectors to run: "+strings.Join(KnownCollectors(), ", "))
	f.StringVar(&since, "since", "", "warning-event window (default 1h)")
	f.IntVar(&limit, "limit", 0, "maximum items per section")

	c.PreRun = func(*cobra.Command, []string) {
		set := func(key, value string) {
			if value != "" {
				(*params)[key] = value
			}
		}
		set("context", kubeContext)
		set("since", since)
		set("what", strings.Join(what, ","))
		if allNamespaces {
			namespace = ""
		}
		set("namespace", namespace)
		if limit > 0 {
			set("limit", strconv.Itoa(limit))
		}
	}
}
