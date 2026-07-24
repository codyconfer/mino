package cmd

import "github.com/spf13/cobra"

func newCalendarCmd() *cobra.Command {
	c := sourceCmd("calendar", "Upcoming Google Calendar events")
	c.Aliases = []string{"cal"}
	return c
}
