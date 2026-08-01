package tmux

import (
	"slices"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name    string
		opts    SplitOpts
		envFlag bool
		want    []string
	}{
		{
			name: "vertical default",
			opts: SplitOpts{Argv: []string{"mino", "pane", "inbox"}},
			want: []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-v", "--", "mino", "pane", "inbox"},
		},
		{
			name: "horizontal with target and absolute size",
			opts: SplitOpts{Target: "%3", Horizontal: true, Size: 90, Argv: []string{"sh"}},
			want: []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-l", "90", "-t", "%3", "--", "sh"},
		},
		{
			name:    "env via -e flag",
			opts:    SplitOpts{Env: []string{"MINO_PANE_OWNER=42"}, Argv: []string{"mino", "pane", "view", "/tmp/a.json"}},
			envFlag: true,
			want: []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-v",
				"-e", "MINO_PANE_OWNER=42", "--", "mino", "pane", "view", "/tmp/a.json"},
		},
		{
			name:    "env falls back to env prefix",
			opts:    SplitOpts{Env: []string{"MINO_PANE_OWNER=42"}, Argv: []string{"mino", "pane", "view", "/tmp/a.json"}},
			envFlag: false,
			want: []string{"split-window", "-d", "-P", "-F", "#{pane_id}", "-v",
				"--", "env", "MINO_PANE_OWNER=42", "mino", "pane", "view", "/tmp/a.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitArgs(tt.opts, tt.envFlag)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("SplitArgs()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestSplitArgsDoesNotAliasArgv(t *testing.T) {
	argv := make([]string, 3, 8)
	copy(argv, []string{"mino", "pane", "inbox"})
	o := SplitOpts{Env: []string{"K=V"}, Argv: argv}
	SplitArgs(o, false)
	if !slices.Equal(argv, []string{"mino", "pane", "inbox"}) {
		t.Fatalf("SplitArgs mutated caller argv: %q", argv)
	}
}
