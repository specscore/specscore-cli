// Package exitcode defines the shared exit code constants and error type
// used by all SpecScore CLI commands and library consumers.
package exitcode

import "fmt"

// Standard exit codes shared by every CLI command.
const (
	Success            = 0  // Operation completed successfully.
	Conflict           = 1  // Concurrent-modification conflict.
	InvalidArgs        = 2  // Missing or invalid command arguments/flags.
	NotFound           = 3  // Requested resource does not exist.
	InvalidState       = 4  // State transition is not allowed.
	AmbiguousSlug      = 5  // Slug auto-resolution found multiple candidates.
	TargetNotSpecScore = 6  // Target directory is not a SpecScore-managed repo.
	DirtyTree          = 7  // Working tree has uncommitted changes in paths to be modified.
	UnsupportedCommand = 8  // Subcommand not recognized — typically an outdated specscore that predates a required subcommand. Distinct from the shell's 127 (binary absent).
	UpdateFailed       = 9  // A self-update could not be completed locally (extraction, staging, or the swap itself). See the note below on why this is not Unexpected.
	Unexpected         = 10 // Catch-all for unexpected runtime errors, EXCEPT in `self-update --check`, where 10 means "an update is available" (see below).
)

// Why UpdateFailed exists.
//
// `self-update --check` reports an available (or undetermined) version by
// exiting 10 — a documented part of that command's contract since it shipped,
// and the number scripts already branch on. But 10 is also Unexpected, this
// package's catch-all, so a local failure during an update (a bad archive, a
// staging error) and "there is a newer release" were indistinguishable to a
// caller: the exact collision `self-update`'s spec forbids.
//
// 9 was the one unassigned number in this vocabulary, so the rarer meaning
// takes it. Moving "update available" off the catch-all instead would be the
// tidier fix, and is the one to make if this contract is ever revised — it
// would break every existing caller that checks for 10, which is why it was
// not done here.

// Error carries a machine-readable exit code alongside a human-readable
// message. It satisfies both the error interface and the ExitCode()
// convention checked by the top-level CLI runner.
type Error struct {
	code  int
	msg   string
	cause error
}

func (e *Error) Error() string { return e.msg }

// ExitCode returns the numeric exit code for this error.
func (e *Error) ExitCode() int { return e.code }

// Unwrap preserves an underlying runtime cause when a command maps it to a
// stable process exit code. Argument/state errors commonly have no cause.
func (e *Error) Unwrap() error { return e.cause }

// New creates an Error with the given exit code and message.
func New(code int, msg string) *Error {
	return &Error{code: code, msg: msg}
}

// Newf creates an Error with the given exit code and formatted message.
func Newf(code int, format string, args ...any) *Error {
	return &Error{code: code, msg: fmt.Sprintf(format, args...)}
}

// Wrap maps cause to code while retaining errors.Is/errors.As identity.
func Wrap(code int, msg string, cause error) *Error {
	return &Error{code: code, msg: msg, cause: cause}
}

// --- Convenience constructors for standard exit codes ---

// ConflictError returns an exit-code-1 error.
func ConflictError(msg string) *Error { return &Error{code: Conflict, msg: msg} }

// ConflictErrorf returns an exit-code-1 error with a formatted message.
func ConflictErrorf(format string, args ...any) *Error {
	return Newf(Conflict, format, args...)
}

// InvalidArgsError returns an exit-code-2 error.
func InvalidArgsError(msg string) *Error { return &Error{code: InvalidArgs, msg: msg} }

// InvalidArgsErrorf returns an exit-code-2 error with a formatted message.
func InvalidArgsErrorf(format string, args ...any) *Error {
	return Newf(InvalidArgs, format, args...)
}

// NotFoundError returns an exit-code-3 error.
func NotFoundError(msg string) *Error { return &Error{code: NotFound, msg: msg} }

// NotFoundErrorf returns an exit-code-3 error with a formatted message.
func NotFoundErrorf(format string, args ...any) *Error {
	return Newf(NotFound, format, args...)
}

// InvalidStateError returns an exit-code-4 error.
func InvalidStateError(msg string) *Error { return &Error{code: InvalidState, msg: msg} }

// InvalidStateErrorf returns an exit-code-4 error with a formatted message.
func InvalidStateErrorf(format string, args ...any) *Error {
	return Newf(InvalidState, format, args...)
}

// UnsupportedCommandError returns an exit-code-8 error.
func UnsupportedCommandError(msg string) *Error { return &Error{code: UnsupportedCommand, msg: msg} }

// UnexpectedError returns an exit-code-10 error.
func UnexpectedError(msg string) *Error { return &Error{code: Unexpected, msg: msg} }

// UnexpectedErrorf returns an exit-code-10 error with a formatted message.
func UnexpectedErrorf(format string, args ...any) *Error {
	return Newf(Unexpected, format, args...)
}

// UnexpectedErrorCause maps an unexpected runtime cause to exit 10 without
// discarding its identity.
func UnexpectedErrorCause(msg string, cause error) *Error {
	return Wrap(Unexpected, msg, cause)
}
