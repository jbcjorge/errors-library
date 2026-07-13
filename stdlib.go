package errors

import nerrors "errors"

// Is reports whether any error in err's tree matches target.
// This is a re-export of [errors.Is] from the standard library.
func Is(err, target error) bool { return nerrors.Is(err, target) }

// As finds the first error in err's tree that matches target, and if one is
// found, sets target to that error value and returns true.
// This is a re-export of [errors.As] from the standard library.
func As(err error, target any) bool { return nerrors.As(err, target) }

// Unwrap returns the result of calling the Unwrap method on err, if err's type
// implements Unwrap. Otherwise, Unwrap returns nil.
// This is a re-export of [errors.Unwrap] from the standard library.
func Unwrap(err error) error { return nerrors.Unwrap(err) }

// Join returns an error that wraps the given errors. Any nil error values are
// discarded. The error formats as the concatenation of the strings obtained by
// calling the Error method of each element of errs, with a newline between
// each message.
// This is a re-export of [errors.Join] from the standard library.
func Join(errs ...error) error { return nerrors.Join(errs...) }
