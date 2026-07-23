package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/active"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/gcal"
)

func newCalendarCmd() *cobra.Command {
	c := sourceCmd("calendar", "Upcoming Google Calendar events")
	c.Aliases = []string{"cal"}
	return c
}

func buildCalendar(params map[string]string) (signals.Signal, error) {
	calID := paramStr(params, "calendar_id", shared.cfg.Cal.CalendarID)
	window := paramDuration(params, "window", parseWindow(shared.cfg.Cal.Window, 24*time.Hour))
	max := paramInt(params, "max", shared.cfg.Cal.Max)
	return gcal.New(calID, window, max, googleAuth()), nil
}

func parseWindow(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func buildActiveCalendar(params map[string]string, state *active.State) (signals.ActiveSignal, error) {
	calID := paramStr(params, "calendar_id", shared.cfg.Cal.CalendarID)
	interval := paramDuration(params, "interval", 60*time.Second)
	return gcal.NewActive(calID, googleAuth(), interval, state), nil
}
