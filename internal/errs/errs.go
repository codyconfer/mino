package errs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/render/glyph"
)

type Kind string

const (
	KindUsage    Kind = "usage"
	KindConfig   Kind = "config"
	KindAuth     Kind = "auth"
	KindStore    Kind = "store"
	KindSignal   Kind = "signal"
	KindBackup   Kind = "backup"
	KindInternal Kind = "internal"

	KindOnboarding Kind = "onboarding"
)

type Error struct {
	Kind  Kind
	Msg   string
	Hint  string
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Cause }

func New(kind Kind, msg string) *Error { return &Error{Kind: kind, Msg: msg} }

func Newf(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

func Wrap(kind Kind, cause error, msg string) *Error {
	if cause == nil {
		return nil
	}
	return &Error{Kind: kind, Msg: msg, Cause: cause}
}

func Wrapf(kind Kind, cause error, format string, args ...any) *Error {
	return Wrap(kind, cause, fmt.Sprintf(format, args...))
}

func (e *Error) WithHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

func Hint(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Hint
	}
	return ""
}

func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}

func Render(err error) string {
	if err == nil {
		return ""
	}
	r := log.Renderer()
	mark := r.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render(glyph.Cross())
	kindSty := r.NewStyle().Faint(true)
	hintSty := r.NewStyle().Foreground(lipgloss.Color("12"))

	var b strings.Builder
	var e *Error
	if errors.As(err, &e) {
		fmt.Fprintf(&b, "%s %s %s\n", mark, kindSty.Render("["+string(e.Kind)+"]"), err.Error())
		if e.Hint != "" {
			fmt.Fprintf(&b, "  %s %s\n", hintSty.Render("hint:"), e.Hint)
		}
		return b.String()
	}
	fmt.Fprintf(&b, "%s %s\n", mark, err.Error())
	return b.String()
}
