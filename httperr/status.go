package httperr

import "github.com/jbcjorge/errors-library"

// DefaultStatus is used when no status can be resolved from an error, for
// example a foreign error or a structured error without [errors.WithHTTPStatus].
const DefaultStatus = 500

// maxVisited bounds how many errors StatusOf inspects, so that a cyclic or
// pathologically branching error tree cannot hang the caller.
const maxVisited = 10_000

// StatusOf resolves the HTTP status code for err. It walks the error tree,
// outer first, and returns the status of the first [*errors.Error] that
// declares a non-zero one via [errors.WithHTTPStatus]. If err is nil, or no
// structured error in the tree declares a status, it returns [DefaultStatus].
//
// The outermost declaration wins: wrapping an error with a new status
// overrides a status set deeper in the chain. A structured error that declares
// no status does not stop the search; its wrapped cause(s) are inspected too.
//
// The search inspects at most 10,000 errors, so error trees that contain a
// cycle resolve to [DefaultStatus] instead of hanging.
func StatusOf(err error) int {
	budget := maxVisited
	if code, ok := findStatus(err, &budget); ok {
		return code
	}
	return DefaultStatus
}

// findStatus walks err outer to inner, descending through both single-unwrap
// (Unwrap() error) and multi-unwrap (Unwrap() []error) wrappers.
func findStatus(err error, budget *int) (int, bool) {
	for err != nil {
		if *budget <= 0 {
			return 0, false
		}
		*budget--

		switch e := err.(type) {
		case *errors.Error:
			if e == nil {
				return 0, false // typed nil: nothing to inspect, calling its methods would panic
			}
			if e.HTTPStatus() != 0 {
				return e.HTTPStatus(), true
			}
		case errors.Error:
			if e.HTTPStatus() != 0 {
				return e.HTTPStatus(), true
			}
		}

		switch u := err.(type) {
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		case interface{ Unwrap() []error }:
			for _, child := range u.Unwrap() {
				if code, ok := findStatus(child, budget); ok {
					return code, true
				}
			}
			return 0, false
		default:
			return 0, false
		}
	}
	return 0, false
}
