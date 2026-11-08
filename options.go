package errors

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
)

// invalidVerbRegex detects the markers fmt.Sprintf writes for a malformed
// format, such as a missing or extra argument or a verb that does not fit the
// argument type (e.g., "%!s(MISSING)", "%!(EXTRA string=x)", "%!d(string=x)").
var invalidVerbRegex = regexp.MustCompile(`%!.?\([^\)]+\)`)

// formatChecked formats tmpl with args and reports whether the formatting was
// well formed. A formatting error marker only counts when it was produced by
// fmt itself: a marker that appears verbatim in the text of an argument comes
// from the caller's data and is not a formatting error.
func formatChecked(tmpl string, args []any) (string, bool) {
	out := fmt.Sprintf(tmpl, args...)
	for _, marker := range invalidVerbRegex.FindAllString(out, -1) {
		if !appearsInArguments(marker, args) {
			return "", false
		}
	}
	return out, true
}

// appearsInArguments reports whether marker is part of the text of any argument.
func appearsInArguments(marker string, args []any) bool {
	for _, arg := range args {
		if strings.Contains(fmt.Sprint(arg), marker) {
			return true
		}
	}
	return false
}

// WithError wraps an existing error inside the [Error] as an original cause.
// The wrapped error is appended immediately and can be retrieved via
// [Error.Unwrap] or [Error.Original]. A nil error is ignored.
func WithError(err error) StoredError {
	return ImmediateParse(
		TypeErrWrapped,
		func(e *Error) {
			if err != nil {
				e.originals = append(e.originals, err)
			}
		})
}

// WithErrors wraps multiple errors inside the [Error]. All wrapped errors are
// walkable via [errors.Is] and [errors.As] through the multi-unwrap interface.
// Nil errors are ignored.
func WithErrors(errs ...error) StoredError {
	return ImmediateParse(
		TypeErrWrapped,
		func(e *Error) {
			for _, err := range errs {
				if err != nil {
					e.originals = append(e.originals, err)
				}
			}
		})
}

// WithParsedMessage supplies printf-style arguments for the error's message
// template. It is late-parsed: the formatting is deferred until [Error.Error]
// is called. If the formatting is malformed (missing or extra arguments, or a
// verb that does not fit its argument), the original template is kept
// unchanged. A literal percent sign in a template must be written as "%%".
func WithParsedMessage(args ...any) StoredError {
	return LateParse(
		TypeErrMessage,
		func(e *Error) {
			e.parsedMessage = e.message
			if parsedMessage, ok := formatChecked(e.message, args); ok {
				e.parsedMessage = parsedMessage
			}
		})
}

// WithSafeParsedMessage supplies printf-style arguments that are safe to
// include in telemetry. When provided, [Error.SafeError] and [Error.SafeMap]
// use the formatted result instead of stripping placeholders. Use this when
// the interpolated values do not contain PII (e.g., error codes, resource
// types, public identifiers). Malformed formatting is handled as in
// [WithParsedMessage]: the safe message falls back to the stripped template.
func WithSafeParsedMessage(args ...any) StoredError {
	return LateParse(
		TypeErrSafeMessage,
		func(e *Error) {
			if parsedMessage, ok := formatChecked(e.message, args); ok {
				e.safeMessage = parsedMessage
			}
		})
}

// WithAdditionalData merges the provided key-value pairs into the error's
// additional data map. This data is considered UNSAFE (may contain PII) and
// is excluded from [Error.SafeMap].
//
// The keys "message", "stackTrace", "originalError", and "originalErrors" are
// reserved. They are produced by the error itself and are ignored if present
// in the data. Values are stored as given and shared with copies made by
// [Error.Parse]; do not mutate maps or slices after passing them in.
func WithAdditionalData(data map[string]any) StoredError {
	return ImmediateParse(
		TypeErrAdditionalData,
		func(e *Error) {
			maps.Copy(e.additionalData, data)
		})
}

// WithSafeData merges the provided key-value pairs into the error's safe data
// map. Safe data is guaranteed to be free of PII and is included in both
// [Error.AsMap] and [Error.SafeMap]. Use this for data that can safely be sent
// to telemetry, logging, and error reporting services.
//
// The keys "message", "stackTrace", "originalError", and "originalErrors" are
// reserved. They are produced by the error itself and are ignored if present
// in the data. Values are stored as given and shared with copies made by
// [Error.Parse]; do not mutate maps or slices after passing them in.
func WithSafeData(data map[string]any) StoredError {
	return ImmediateParse(
		TypeErrSafeData,
		func(e *Error) {
			maps.Copy(e.safeData, data)
		})
}

// WithJSONOutputAsUnsafe makes [Error.MarshalJSON] encode the internal view
// ([Error.AsMap]) instead of the safe one. The internal view includes
// additional data, interpolated arguments, and the text of wrapped errors, and
// may contain PII.
//
// Safe output is the default; this is the explicit opt-out, for errors whose
// JSON goes only to internal sinks. To limit the opt-out to one occurrence,
// pass it to [Error.Parse] instead of New: Parse returns a fresh error, so a
// sentinel keeps its safe default.
//
// It has no effect on [Error.SafeMap], [Error.SafeError], or the httperr
// subpackage, which always use the safe view.
func WithJSONOutputAsUnsafe() StoredError {
	return ImmediateParse(
		TypeErrUnsafeJSON,
		func(e *Error) {
			e.unsafeJSON = true
		})
}

// WithGeneralOutputAsUnsafe makes every implicit output of the error use the
// internal view ([Error.AsMap]) instead of the safe one. An implicit output is
// one the caller does not ask for by name; today that is [Error.MarshalJSON],
// and outputs added later will respect this option as well.
//
// It is a superset of [WithJSONOutputAsUnsafe]. The same rules apply: safe is
// the default, this is an explicit opt-out for internal sinks, it can be scoped
// to one occurrence by passing it to [Error.Parse], and it has no effect on
// [Error.SafeMap], [Error.SafeError], or the httperr subpackage.
func WithGeneralOutputAsUnsafe() StoredError {
	return ImmediateParse(
		TypeErrUnsafeOutput,
		func(e *Error) {
			e.unsafeOutput = true
		})
}

// WithHTTPStatus associates an HTTP status code with the error. The code is
// stored as structured metadata (it does not affect the rendered message) and
// can be read back via [Error.HTTPStatus]. The companion httperr subpackage
// resolves the status across an error chain. This is typically set at sentinel
// declaration time, since the status is an invariant property of the error
// kind:
//
//	var ErrNotAuthorized = errors.New("not authorized",
//		errors.WithHTTPStatus(http.StatusForbidden))
func WithHTTPStatus(code int) StoredError {
	return ImmediateParse(
		TypeErrHTTPStatus,
		func(e *Error) {
			e.httpStatus = code
		})
}
