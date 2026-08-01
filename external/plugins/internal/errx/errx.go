package errx

import (
	"strings"
	"unicode"

	"github.com/codyconfer/mino/plugin"
)

type Error = plugin.Error

func New(msg string) *Error { return plugin.NewError(msg) }

func Newf(format string, args ...any) *Error { return plugin.NewErrorf(format, args...) }

func Wrap(cause error, msg string) *Error { return plugin.WrapError(cause, msg) }

func Wrapf(cause error, format string, args ...any) *Error {
	return plugin.WrapErrorf(cause, format, args...)
}

func ExcerptBytes(body []byte) string {
	const limit = 200
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, string(body))
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > limit {
		return clean[:limit] + "…"
	}
	return clean
}
