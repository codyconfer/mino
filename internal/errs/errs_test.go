package errs_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/plugin"
)

const hostileBody = "\x1b]0;pwned\a\x1b[2J\x1b[32mall checks passed\x1b[0m\x7f rate limit"

func hasTerminalEscapes(s string) bool {
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case r == 0x1b, r == 0x9b, r == 0x07, r == 0x7f, r == '\r':
			return true
		case r < 0x20:
			return true
		}
	}
	return false
}

func TestRenderStripsEscapesFromRemoteText(t *testing.T) {
	err := errs.Newf(errs.KindSignal, "github api 403 Forbidden: %s", hostileBody)
	got := errs.Render(err)
	if hasTerminalEscapes(got) {
		t.Fatalf("Render leaked terminal control sequences: %q", got)
	}
	if !strings.Contains(got, "all checks passed") {
		t.Fatalf("Render dropped the readable text: %q", got)
	}
}

func TestRenderStripsEscapesFromHints(t *testing.T) {
	err := errs.New(errs.KindAuth, "bad token").WithHint("scope %s is required", "\x1b[31mread:org\x07")
	got := errs.Render(err)
	if hasTerminalEscapes(got) {
		t.Fatalf("Render leaked terminal control sequences from the hint: %q", got)
	}
}

func TestRenderStripsEscapesFromPlainErrors(t *testing.T) {
	got := errs.Render(errors.New("boom \x1b]0;pwned\a"))
	if hasTerminalEscapes(got) {
		t.Fatalf("Render leaked terminal control sequences from a plain error: %q", got)
	}
}

func renderedMark(t *testing.T) string {
	t.Helper()
	got := errs.Render(errors.New("x"))
	mark := strings.TrimSuffix(got, " x\n")
	if mark == got || mark == "" {
		t.Fatalf("could not isolate the leading mark from %q", got)
	}
	return mark
}

func TestRenderPluginErrorHintOnceInErrsStyle(t *testing.T) {
	mark := renderedMark(t)
	err := plugin.NewError("no Slack token available").WithHint("run `mino login slack`")
	want := mark + " no Slack token available\n  hint: run `mino login slack`\n"
	got := errs.Render(err)
	if got != want {
		t.Fatalf("Render mismatch\n got: %q\nwant: %q", got, want)
	}
	if strings.Count(got, "hint:") != 1 {
		t.Fatalf("hint rendered more than once: %q", got)
	}
}

func TestRenderWrappedPluginErrorJoinsHints(t *testing.T) {
	inner := plugin.NewError("no Slack token available").WithHint("export a user token")
	outer := plugin.WrapError(inner, "slack authentication").WithHint("set SLACK_TOKEN")
	got := errs.Render(outer)
	if strings.Count(got, "hint:") != 1 {
		t.Fatalf("want exactly one hint marker, got %q", got)
	}
	if !strings.Contains(got, "set SLACK_TOKEN") || !strings.Contains(got, "export a user token") {
		t.Fatalf("Render dropped a chained hint: %q", got)
	}
	if !strings.Contains(got, "slack authentication: no Slack token available\n") {
		t.Fatalf("Render dropped the message chain: %q", got)
	}
}

func TestRenderKeepsMinosOwnMessagesByteForByte(t *testing.T) {
	mark := renderedMark(t)
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "kind and hint",
			err:  errs.New(errs.KindConfig, "missing Google OAuth desktop-app client credentials").WithHint("set `google.oauth_client_id` and `google.oauth_client_secret` in config to use `mino login google`"),
			want: " [config] missing Google OAuth desktop-app client credentials\n  hint: set `google.oauth_client_id` and `google.oauth_client_secret` in config to use `mino login google`\n",
		},
		{
			name: "multiline hint",
			err: errs.New(errs.KindAuth, "no Application Default Credentials found").WithHint(
				"authorize Google access with either:\n  gcloud auth application-default login \\\n    --scopes=%s\nor:\n  mino login google", "openid,email"),
			want: " [auth] no Application Default Credentials found\n  hint: authorize Google access with either:\n  gcloud auth application-default login \\\n    --scopes=openid,email\nor:\n  mino login google\n",
		},
		{
			name: "wrapped cause",
			err:  errs.Wrap(errs.KindStore, errors.New("permission denied"), "opening the token store"),
			want: " [store] opening the token store: permission denied\n",
		},
		{
			name: "plain error",
			err:  errors.New("something went wrong"),
			want: " something went wrong\n",
		},
		{
			name: "tab in message",
			err:  errs.New(errs.KindUsage, "columns:\ta\tb"),
			want: " [usage] columns:\ta\tb\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := mark + tc.want
			if got := errs.Render(tc.err); got != want {
				t.Fatalf("Render mismatch\n got: %q\nwant: %q", got, want)
			}
		})
	}
}
