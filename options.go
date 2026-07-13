package errors

import (
	"fmt"
	"maps"
	"regexp"
)

// invalidVerbRegex detects malformed printf verbs produced by fmt.Sprintf when
// arguments are missing or mismatched (e.g., "%!s(MISSING)").
var invalidVerbRegex = regexp.MustCompile(`%!.?\([^\)]+\)`)

// WithError wraps an existing error inside the [Error] as an original cause.
// The wrapped error is appended immediately and can be retrieved via
// [Error.Unwrap] or [Error.Original].
func WithError(err error) StoredError {
	return ImmediateParse(
		TypeErrWrapped,
		func(e *Error) {
			e.originals = append(e.originals, err)
		})
}

// WithErrors wraps multiple errors inside the [Error]. All wrapped errors are
// walkable via [errors.Is] and [errors.As] through the multi-unwrap interface.
func WithErrors(errs ...error) StoredError {
	return ImmediateParse(
		TypeErrWrapped,
		func(e *Error) {
			e.originals = append(e.originals, errs...)
		})
}

// WithParsedMessage supplies printf-style arguments for the error's message
// template. It is late-parsed: the formatting is deferred until [Error.Error]
// is called. If formatting produces invalid verbs (e.g. missing arguments),
// the original template is kept unchanged.
func WithParsedMessage(args ...any) StoredError {
	return LateParse(
		TypeErrMessage,
		func(e *Error) {
			e.parsedMessage = e.message
			parsedMessage := fmt.Sprintf(e.message, args...)
			if !invalidVerbRegex.Match([]byte(parsedMessage)) {
				e.parsedMessage = parsedMessage
			}
		})
}

// WithSafeParsedMessage supplies printf-style arguments that are safe to
// include in telemetry. When provided, [Error.SafeError] and [Error.SafeMap]
// use the formatted result instead of stripping placeholders. Use this when
// the interpolated values do not contain PII (e.g., error codes, resource
// types, public identifiers).
func WithSafeParsedMessage(args ...any) StoredError {
	return LateParse(
		TypeErrSafeMessage,
		func(e *Error) {
			parsedMessage := fmt.Sprintf(e.message, args...)
			if !invalidVerbRegex.Match([]byte(parsedMessage)) {
				e.safeMessage = parsedMessage
			}
		})
}

// WithAdditionalData merges the provided key-value pairs into the error's
// additional data map. This data is considered UNSAFE (may contain PII) and
// is excluded from [Error.SafeMap].
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
func WithSafeData(data map[string]any) StoredError {
	return ImmediateParse(
		TypeErrSafeData,
		func(e *Error) {
			maps.Copy(e.safeData, data)
		})
}
