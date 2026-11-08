// Package httperr writes structured [errors.Error] values as HTTP error
// responses.
//
// A [Writer] is bound to one http.ResponseWriter and configured with options.
// Calling [Writer.Write] unpacks the error, builds a [Payload] from its
// telemetry-safe representation, validates it, and hands it to a [Renderer]
// that decides the wire format. [JSON] is the default renderer and
// [ProblemJSON] (RFC 9457) is built in; any type implementing [Renderer] can
// be plugged in with [WithRenderer].
//
//	var errOpts = []httperr.Option{httperr.WithRenderer(httperr.JSON(httperr.MessageKey("error")))}
//
//	writer := httperr.NewWriter(w, errOpts...)
//	if err := writer.Write(appErr); err != nil {
//		// nothing was sent unless the error is ErrWrite: log it, then
//		_ = writer.Fallback() // answer with a bare 500 instead of an empty 200
//	}
//
// The payload is built only from data the error's author marked as safe
// ([errors.Error.SafeMap]), so data attached with errors.WithAdditionalData
// never reaches the client, and wrapped errors are never sent. Errors that are
// not an [*errors.Error] carry no safety classification, so their text is never
// forwarded: they are turned into a native error with [WithConverter], or
// answered with a 500 and the fallback message ([WithFallbackMessage]).
//
// This package imports net/http. Code that does not serve HTTP responses
// should depend only on the core errors package.
package httperr

import "github.com/jbcjorge/errors-library"

// Sentinels returned by [Writer.Write]. Nothing has been sent to the client
// when Write returns ErrNothingToWrite, ErrEmptyMessage, ErrInvalidStatus,
// ErrRender, or ErrAlreadyWritten, so the caller can still respond, at the
// very least with [Writer.Fallback]. When it returns ErrWrite the status line
// is already on the wire and the caller can only log.
var (
	// ErrNothingToWrite is returned when Write receives a nil error.
	ErrNothingToWrite = errors.New("nothing to write: the error is nil")
	// ErrEmptyMessage is returned when the error's safe message is empty or
	// whitespace only (for example a "%s" template, which sanitizes to "").
	ErrEmptyMessage = errors.New("nothing to write: the error message is empty")
	// ErrInvalidStatus is returned when the resolved status is outside 400 to 599.
	ErrInvalidStatus = errors.New("invalid status %d for an error response, expected 400 to 599")
	// ErrRender is returned when the renderer fails. The cause is wrapped.
	ErrRender = errors.New("rendering the error response failed")
	// ErrWrite is returned when writing the rendered body fails after the
	// status line was sent. The cause is wrapped.
	ErrWrite = errors.New("writing the error response failed")
	// ErrAlreadyWritten is returned when Write is called again on a Writer
	// that already sent a response.
	ErrAlreadyWritten = errors.New("an error response was already written")
)
