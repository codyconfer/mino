//go:build nodaemon

package cmd

import "testing"

func TestNoDaemonCommandsRegistered(t *testing.T) {
	root := Root()
	for _, n := range []string{"serve", "daemon"} {
		if findCmd(root, n) != nil {
			t.Errorf("command %q must not exist in a nodaemon build", n)
		}
	}
}
