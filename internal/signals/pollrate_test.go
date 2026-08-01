package signals

import (
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func TestCheckPollIntervalRejectsBelowTheFloor(t *testing.T) {
	for _, d := range []time.Duration{-time.Second, 0, time.Nanosecond, 100 * time.Millisecond, 999 * time.Millisecond} {
		err := CheckPollInterval("probe", d)
		if err == nil {
			t.Errorf("CheckPollInterval(%s) = nil; a sub-second poll rate hot-polls the provider "+
				"straight into a rate limit, so anything under %s must be refused", d, MinPollInterval)
			continue
		}
		if errs.KindOf(err) != errs.KindUsage {
			t.Errorf("CheckPollInterval(%s) kind = %v, want %v so the CLI prints usage rather than a stack",
				d, errs.KindOf(err), errs.KindUsage)
		}
	}
}

func TestPollIntervalFloorHintMentionsGitHubFloor(t *testing.T) {
	want := "polling faster than " + MinPollInterval.String() +
		" burns provider rate limits; GitHub's own X-Poll-Interval floor is 60s"
	if got := errs.Hint(CheckPollInterval("probe", 100*time.Millisecond)); got != want {
		t.Errorf("CheckPollInterval hint = %q, want %q", got, want)
	}
	_, err := ParsePollInterval("probe", "100ms")
	if got := errs.Hint(err); got != want {
		t.Errorf("ParsePollInterval below-floor hint = %q, want %q", got, want)
	}
	_, err = ParsePollInterval("probe", "1minute")
	if got, wantParse := errs.Hint(err), "use a Go duration such as 30s, 2m, or 1h"; got != wantParse {
		t.Errorf("ParsePollInterval malformed hint = %q, want %q", got, wantParse)
	}
}

func TestCheckPollIntervalAcceptsTheFloorAndAbove(t *testing.T) {
	for _, d := range []time.Duration{MinPollInterval, 30 * time.Second, time.Hour} {
		if err := CheckPollInterval("probe", d); err != nil {
			t.Errorf("CheckPollInterval(%s) = %v, want nil", d, err)
		}
	}
}

func TestParsePollIntervalRejectsMalformedAndBelowFloor(t *testing.T) {
	if _, err := ParsePollInterval("probe", "1minute"); err == nil {
		t.Error(`ParsePollInterval("1minute") = nil; a malformed duration used to fall back to the ` +
			`default silently, which hides the typo`)
	}
	if _, err := ParsePollInterval("probe", "100ms"); err == nil {
		t.Error(`ParsePollInterval("100ms") = nil; want the floor enforced on parsed values too`)
	}
	got, err := ParsePollInterval("probe", "90s")
	if err != nil || got != 90*time.Second {
		t.Errorf(`ParsePollInterval("90s") = %s, %v; want 1m30s, nil`, got, err)
	}
}
