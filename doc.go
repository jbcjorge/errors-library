// Package errors provides structured, composable error handling.
//
// The central type is [Error], which wraps a message template with optional
// late-parsed format arguments, an original (wrapped) error, and arbitrary
// additional data. Errors are built with [New] and customised with option
// functions such as [WithError], [WithParsedMessage], and [WithAdditionalData].
//
// Options can be applied immediately (at construction time) or deferred until
// the error message is rendered (late-parse). Late-parsed options are evaluated
// lazily when [Error.Error] is called, which allows format arguments to be
// supplied after the error template is created.
//
// Error implements the standard [error], [json.Marshaler], and
// [errors.Is]/[errors.Unwrap] interfaces for seamless interoperability with
// Go's error handling ecosystem.
//
// The package also re-exports standard library functions and values ([Is],
// [As], [AsType], [Unwrap], [Join], [ErrUnsupported]) so it can serve as a
// single import replacing the standard "errors" package.
//
// # HTTP status codes
//
// An [Error] can carry an HTTP status code via [WithHTTPStatus]. The code is
// structured metadata that does not affect the rendered message; read it back
// with [Error.HTTPStatus]. This package only stores the number and knows
// nothing else about HTTP. The companion subpackage "httperr" resolves the
// status across an error chain and serializes the error as an HTTP response
// built from the telemetry-safe [Error.SafeMap].
package errors
