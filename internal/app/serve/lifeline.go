package serve

import (
	"context"
	"io"
	"os"
	"strconv"
)

const deckLifelineEnv = "MUNIN_DECK_LIFELINE"

func BindDeckLifeline(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	fdStr := os.Getenv(deckLifelineEnv)
	if fdStr == "" {
		return ctx, cancel
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil || fd < 3 {
		return ctx, cancel
	}
	f := os.NewFile(uintptr(fd), "deck-lifeline")
	if f == nil {
		return ctx, cancel
	}
	go func() {
		defer f.Close()
		_, _ = io.Copy(io.Discard, f)
		cancel()
	}()
	return ctx, cancel
}
