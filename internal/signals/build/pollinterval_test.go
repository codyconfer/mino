package build

import (
	"testing"
	"time"
)

func TestParamPollIntervalRefusesAHotPollRate(t *testing.T) {
	for _, raw := range []string{"0s", "1ms", "100ms", "999ms"} {
		if _, err := paramPollInterval(map[string]string{"interval": raw}, "probe", time.Minute); err == nil {
			t.Errorf("paramPollInterval(interval=%s) = nil; the param used to accept any parseable duration, "+
				"so `interval: %s` hot-polled the provider on every tick", raw, raw)
		}
	}
}

func TestParamPollIntervalRefusesAMalformedDuration(t *testing.T) {
	if _, err := paramPollInterval(map[string]string{"interval": "1minute"}, "probe", time.Minute); err == nil {
		t.Error("paramPollInterval(interval=1minute) = nil; an unparseable duration used to fall back to the " +
			"default silently, so the typo never surfaced")
	}
}

func TestParamPollIntervalKeepsValidRatesAndTheDefault(t *testing.T) {
	got, err := paramPollInterval(map[string]string{"interval": "5m"}, "probe", time.Minute)
	if err != nil || got != 5*time.Minute {
		t.Errorf("paramPollInterval(interval=5m) = %s, %v; want 5m0s, nil", got, err)
	}
	got, err = paramPollInterval(nil, "probe", 90*time.Second)
	if err != nil || got != 90*time.Second {
		t.Errorf("paramPollInterval(no param) = %s, %v; want the default 1m30s, nil", got, err)
	}
}
