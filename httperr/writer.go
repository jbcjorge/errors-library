package httperr

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/jbcjorge/errors-library"
)

// Writer writes one error response to an http.ResponseWriter. Create one per
// request with [NewWriter]. A Writer is not safe for concurrent use.
type Writer struct {
	w       http.ResponseWriter
	cfg     config
	status  int
	written bool
}

// NewWriter returns a Writer bound to w and configured with opts. Options are
// plain values, so a service can declare them once and reuse them:
//
//	var errOpts = []httperr.Option{httperr.WithRenderer(httperr.ProblemJSON())}
//	writer := httperr.NewWriter(w, errOpts...)
func NewWriter(w http.ResponseWriter, opts ...Option) *Writer {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Writer{w: w, cfg: cfg}
}

// Status returns the status code sent by [Writer.Write] or [Writer.Fallback],
// or 0 if no response was sent.
func (wr *Writer) Status() int { return wr.status }

// Write sends err as an error response. It unpacks err into a [Payload],
// validates it, renders it into a buffer, and only then sends the headers and
// body, so a failure before sending leaves the response untouched.
//
// When Write fails before sending, nothing has been written and the caller
// still owns the response: handle the error and, at the very least, call
// [Writer.Fallback], otherwise the client receives an empty 200 response.
//
// It returns nil on success, or one of the package sentinels (match them with
// errors.Is): [ErrAlreadyWritten], [ErrNothingToWrite], [ErrEmptyMessage],
// [ErrInvalidStatus], [ErrRender] (nothing sent), or [ErrWrite] (status sent,
// body failed).
//
// Write sets Content-Type and X-Content-Type-Options, and removes any
// Content-Length set earlier, because the body is not the one the caller
// announced. Other headers are left as they are. A Renderer that panics
// propagates the panic; nothing has been sent at that point.
func (wr *Writer) Write(err error) error {
	if wr.written {
		return ErrAlreadyWritten.Parse()
	}
	if err == nil {
		return ErrNothingToWrite.Parse()
	}

	p := wr.payload(err)
	if verr := validate(p); verr != nil {
		return verr
	}

	var buf bytes.Buffer
	if rerr := wr.cfg.renderer.Render(&buf, p); rerr != nil {
		return ErrRender.Parse(errors.WithError(rerr))
	}
	return wr.send(p.Status, wr.cfg.renderer.ContentType(), buf.Bytes())
}

// Fallback sends a bare 500 response carrying only the fallback message
// ([WithFallbackMessage]) as plain text. It needs no renderer and cannot fail
// validation, so it is the last resort when [Writer.Write] returned an error
// before sending anything. It returns [ErrAlreadyWritten] if a response was
// already sent, or [ErrWrite] if the body could not be written.
func (wr *Writer) Fallback() error {
	if wr.written {
		return ErrAlreadyWritten.Parse()
	}
	msg := wr.cfg.fallbackMessage
	if strings.TrimSpace(msg) == "" {
		msg = http.StatusText(DefaultStatus)
	}
	return wr.send(DefaultStatus, "text/plain; charset=utf-8", []byte(msg+"\n"))
}

// send writes the headers, status line, and body, and records the outcome.
func (wr *Writer) send(status int, contentType string, body []byte) error {
	wr.written = true
	wr.status = status

	header := wr.w.Header()
	header.Set("Content-Type", contentType)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Del("Content-Length")
	wr.w.WriteHeader(status)
	if _, werr := wr.w.Write(body); werr != nil {
		return ErrWrite.Parse(errors.WithError(werr))
	}
	return nil
}

// validate checks that the payload is worth sending.
func validate(p Payload) error {
	if strings.TrimSpace(p.Message) == "" {
		return ErrEmptyMessage.Parse()
	}
	if p.Status < 400 || p.Status > 599 {
		return ErrInvalidStatus.Parse(
			errors.WithParsedMessage(p.Status),
			errors.WithSafeParsedMessage(p.Status),
		)
	}
	return nil
}

// payload unpacks err into a Payload. Native errors (found anywhere in the
// chain, or produced by the converter) contribute their safe message, safe
// data, and status. Anything else becomes a 500 with the fallback message.
// Wrapped errors are never part of the payload.
func (wr *Writer) payload(err error) Payload {
	native, statusSource := wr.resolve(err)
	if native == nil {
		return Payload{
			Status:  DefaultStatus,
			Message: wr.cfg.fallbackMessage,
			Data:    map[string]any{},
		}
	}

	data := native.SafeMap()
	msg, _ := data["message"].(string)
	delete(data, "message")
	delete(data, "originalError")
	delete(data, "originalErrors")
	if !wr.cfg.includeStack {
		delete(data, "stackTrace")
	}

	return Payload{
		Status:  StatusOf(statusSource),
		Message: msg,
		Data:    data,
	}
}

// resolve finds the native error to render and the error to resolve the
// status from. It returns a nil native error when err has no native form.
//
// When the converter produces the native error, the status comes from that
// converted error and not from the foreign original, which carries none.
func (wr *Writer) resolve(err error) (*errors.Error, error) {
	if typed, ok := err.(*errors.Error); ok && typed == nil {
		// A typed nil is a non-nil interface; walking it would call methods on
		// a nil pointer. Treat it as having no native form.
		return nil, nil
	}
	if native, ok := errors.AsType[*errors.Error](err); ok && native != nil {
		return native, err
	}
	if value, ok := errors.AsType[errors.Error](err); ok {
		return &value, err
	}
	if wr.cfg.converter != nil {
		if converted, ok := wr.cfg.converter(err); ok && converted != nil {
			return converted, converted
		}
	}
	return nil, nil
}
