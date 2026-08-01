package plugin

import (
	"fmt"
	"strings"
)

type Error struct {
	msg   string
	hint  string
	cause error
}

func NewError(msg string) *Error { return &Error{msg: msg} }

func NewErrorf(format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

func WrapError(cause error, msg string) *Error { return &Error{msg: msg, cause: cause} }

func WrapErrorf(cause error, format string, args ...any) *Error {
	return &Error{msg: fmt.Sprintf(format, args...), cause: cause}
}

func (e *Error) WithHint(format string, args ...any) *Error {
	e.hint = fmt.Sprintf(format, args...)
	return e
}

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Message())
	for _, hint := range e.hints() {
		b.WriteString("\nhint: ")
		b.WriteString(hint)
	}
	return b.String()
}

func (e *Error) Message() string {
	if e.cause == nil {
		return e.msg
	}
	if inner, ok := e.cause.(*Error); ok {
		return e.msg + ": " + inner.Message()
	}
	return e.msg + ": " + e.cause.Error()
}

func (e *Error) Hint() string {
	return strings.Join(e.hints(), "\n")
}

func (e *Error) hints() []string {
	var out []string
	seen := map[string]bool{}
	for err := error(e); err != nil; {
		wrapped, ok := err.(*Error)
		if !ok {
			break
		}
		if wrapped.hint != "" && !seen[wrapped.hint] {
			seen[wrapped.hint] = true
			out = append(out, wrapped.hint)
		}
		err = wrapped.cause
	}
	return out
}
