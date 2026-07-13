package errors

import (
	"encoding/json"
	nerrors "errors"
	"fmt"
	"maps"
	"regexp"
	"runtime"
	"strings"
)

// New creates an [Error] with the given message template and applies any
// provided options. The message may contain printf-style verbs that are filled
// in later via [WithParsedMessage].
func New(message string, opts ...StoredError) *Error {
	err := &Error{
		message:        message,
		parsedMessage:  message,
		additionalData: map[string]any{},
		safeData:       map[string]any{},
		originals:      nil,
		options:        make(map[ErrorType]ErrorOption),
		stack:          captureStack(2),
	}
	err.AddOptions(opts...)
	return err
}

// parse executes all late-parsed options, filling in deferred format arguments.
func (e *Error) parse() {
	for _, opt := range e.options {
		opt(e)
	}
}

// AddOptions applies the given options to the error. Immediate-parse options
// are executed right away; late-parse options are stored for deferred execution.
func (e *Error) AddOptions(opts ...StoredError) {
	for _, opt := range opts {
		if opt.lateParse {
			e.options[opt.name] = opt.errorFunc
			continue
		}
		opt.errorFunc(e)
	}
}

// Parse creates a copy of the error, applies the given options to the copy, and
// returns it. The original receiver is never mutated, making it safe to use
// package-level sentinel errors with Parse in concurrent code.
// The stack trace is captured at the Parse call site.
func (e *Error) Parse(opts ...StoredError) error {
	clone := e.clone()
	clone.stack = captureStack(2)
	clone.AddOptions(opts...)
	return clone
}

// clone returns a deep copy of the Error.
func (e *Error) clone() *Error {
	newOpts := make(map[ErrorType]ErrorOption, len(e.options))
	for k, v := range e.options {
		newOpts[k] = v
	}
	newData := make(map[string]any, len(e.additionalData))
	maps.Copy(newData, e.additionalData)
	newSafe := make(map[string]any, len(e.safeData))
	maps.Copy(newSafe, e.safeData)
	return &Error{
		message:        e.message,
		parsedMessage:  e.parsedMessage,
		safeMessage:    e.safeMessage,
		originals:      append([]error(nil), e.originals...),
		additionalData: newData,
		safeData:       newSafe,
		options:        newOpts,
		stack:          e.stack,
	}
}

// Error implements the [error] interface. It triggers late-parsed options
// before returning the fully rendered message string.
func (e Error) Error() string {
	e.parse()
	return e.parsedMessage
}

// SafeError returns a PII-free version of the error message. If
// [WithSafeParsedMessage] was used, it returns the safe-formatted message.
// Otherwise, it strips format placeholders from the template
// (e.g., "user %s not found" becomes "user not found").
func (e Error) SafeError() string {
	e.parse()
	if e.safeMessage != "" {
		return e.safeMessage
	}
	return sanitizeTemplate(e.message)
}

// Original returns the first wrapped original error, or the receiver itself if
// no original errors were set. Late-parsed options are triggered before the check.
func (e Error) Original() error {
	if len(e.originals) == 0 {
		e.parse()
		return e
	}
	return e.originals[0]
}

// Originals returns all wrapped original errors.
func (e Error) Originals() []error {
	return e.originals
}

// Is implements the interface used by [errors.Is]. Two Errors are considered
// equal when their raw message templates match.
func (e Error) Is(target error) bool {
	te, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.message == te.message
}

// Unwrap returns all wrapped errors for use with [errors.Is] and [errors.As].
// Implements the multi-unwrap interface (Go 1.20+).
func (e Error) Unwrap() []error {
	return e.originals
}

// AsMap returns a full map representation of the error, including the parsed
// message, all additional data (safe and unsafe), and recursively embedded
// original errors. Do NOT send this to external telemetry if PII may be present.
func (e Error) AsMap() map[string]any {
	temporaryMap := map[string]any{
		"message": e.parsedMessage,
	}
	maps.Copy(temporaryMap, e.safeData)
	maps.Copy(temporaryMap, e.additionalData)

	if len(e.stack) > 0 {
		temporaryMap["stackTrace"] = e.StackTrace()
	}

	if len(e.originals) == 1 {
		temporaryMap["originalError"] = e.originals[0].Error()
		if embeddedError, ok := nerrors.AsType[*Error](e.originals[0]); ok {
			temporaryMap["originalError"] = embeddedError.AsMap()
		}
	} else if len(e.originals) > 1 {
		var errs []any
		for _, orig := range e.originals {
			if embeddedError, ok := nerrors.AsType[*Error](orig); ok {
				errs = append(errs, embeddedError.AsMap())
			} else {
				errs = append(errs, orig.Error())
			}
		}
		temporaryMap["originalErrors"] = errs
	}

	return temporaryMap
}

// SafeMap returns a redacted map representation containing only the message
// template (with format placeholders stripped), safe data, and the stack trace.
// PII-bearing fields (additionalData, parsed arguments) are excluded. Safe to
// send to Sentry, Datadog, or any external telemetry system.
//
// If [WithSafeParsedMessage] was provided, the "message" field contains the
// safe-formatted message. Otherwise, printf verbs are stripped to produce a
// clean, human-readable string (e.g., "user %s not found" becomes "user not found").
func (e Error) SafeMap() map[string]any {
	e.parse()
	msg := sanitizeTemplate(e.message)
	if e.safeMessage != "" {
		msg = e.safeMessage
	}
	safeMap := map[string]any{
		"message": msg,
	}
	maps.Copy(safeMap, e.safeData)

	if len(e.stack) > 0 {
		safeMap["stackTrace"] = e.StackTrace()
	}

	if len(e.originals) == 1 {
		safeMap["originalError"] = e.originals[0].Error()
		if embeddedError, ok := nerrors.AsType[*Error](e.originals[0]); ok {
			safeMap["originalError"] = embeddedError.SafeMap()
		}
	} else if len(e.originals) > 1 {
		var errs []any
		for _, orig := range e.originals {
			if embeddedError, ok := nerrors.AsType[*Error](orig); ok {
				errs = append(errs, embeddedError.SafeMap())
			} else {
				errs = append(errs, orig.Error())
			}
		}
		safeMap["originalErrors"] = errs
	}

	return safeMap
}

// StackTrace returns the captured call stack as a slice of human-readable
// "function file:line" strings.
func (e Error) StackTrace() []string {
	frames := runtime.CallersFrames(e.stack)
	var trace []string
	for {
		frame, more := frames.Next()
		trace = append(trace, fmt.Sprintf("%s %s:%d", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}
	return trace
}

// captureStack records the current call stack, skipping the specified number of
// frames (to exclude captureStack itself and its caller).
func captureStack(skip int) []uintptr {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip+1, pcs)
	return pcs[:n]
}

// sanitizeTemplate strips printf format verbs (e.g., %s, %d, %v, %02d) from a
// message template and collapses any resulting double spaces.
func sanitizeTemplate(msg string) string {
	stripped := fmtVerbRegex.ReplaceAllString(msg, "")
	collapsed := multiSpace.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(collapsed)
}

var (
	fmtVerbRegex = regexp.MustCompile(`%[+\-#0 ]*[0-9]*\.?[0-9]*[a-zA-Z]`)
	multiSpace   = regexp.MustCompile(`\s{2,}`)
)

// MarshalJSON implements [encoding/json.Marshaler] by marshalling the map
// representation returned by [Error.AsMap].
func (e Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.AsMap())
}
