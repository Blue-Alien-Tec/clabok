// Package cli provides the command contract, the JSON envelope and the dispatch loop.
//
// The package owns the output streams, the exit codes and the dry-run gate. It never imports a
// domain package; domain packages register their commands from outside.
package cli

import (
	"errors"
	"fmt"
)

// Kind classifies a command outcome for the exit code.
type Kind int

const (
	// KindFailure is a real failure: unreadable data, a corrupt container, a failed write.
	KindFailure Kind = iota
	// KindUsage is a caller mistake: an unknown command or flag, a missing argument.
	KindUsage
	// KindReject is a gate refusal: nothing was written and the details explain why.
	KindReject
)

// Error is a command outcome with a stable machine code and an optional hint.
type Error struct {
	Code    string
	Message string
	Hint    string
	Details any
	Kind    Kind
}

// Error returns the human message.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Fail reports a real failure.
func Fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: KindFailure}
}

// Usage reports a caller mistake.
func Usage(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: KindUsage}
}

// Reject reports a gate refusal. The change was refused and nothing was written.
func Reject(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Kind: KindReject}
}

// WithHint attaches one actionable sentence.
func (e *Error) WithHint(hint string) *Error {
	e.Hint = hint
	return e
}

// WithDetails attaches structured context for tooling.
func (e *Error) WithDetails(details any) *Error {
	e.Details = details
	return e
}

// AsError maps any error to a command error. An untyped error is an internal failure.
func AsError(err error) *Error {
	var commandErr *Error
	if errors.As(err, &commandErr) && commandErr != nil {
		return commandErr
	}
	return Fail("internal.error", "%v", err)
}
