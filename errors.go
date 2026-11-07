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
// in later via [WithParsedMessage]. A literal percent sign must be written as
// "%%".
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
//
// AddOptions mutates the receiver and is not safe for concurrent use. Never
// call it on a shared package-level sentinel once it may be used by other
// goroutines; use [Error.Parse], which works on a copy, instead.
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
//
// The copy owns its own data maps, wrapped error list, and options. The values
// stored in those maps are shared with the original, so a map or slice passed
// through [WithSafeData] or [WithAdditionalData] must not be mutated afterwards.
func (e *Error) Parse(opts ...StoredError) error {
	clone := e.clone()
	clone.stack = captureStack(2)
	clone.AddOptions(opts...)
	return clone
}

// clone returns a copy of the Error with its own maps, wrapped error list, and
// options. Values stored inside the data maps are not copied.
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

// Original returns the first wrapped original error, or, if no original errors
// were set, a copy of the receiver as an [Error] value (not a pointer).
// Late-parsed options are triggered before the check.
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

// maxMapDepth bounds how deeply AsMap and SafeMap follow wrapped library
// errors. Real error chains are a handful of levels deep; the bound exists so
// that an error graph that wraps itself degrades gracefully (the deeper causes
// are omitted) instead of overflowing the stack, which Go cannot recover from.
const maxMapDepth = 64

// maxMapNodes bounds how many wrapped library errors one AsMap or SafeMap call
// visits in total. The depth bound alone is not enough: an error that wraps
// itself twice branches at every level and would visit 2^64 nodes.
const maxMapNodes = 1000

// AsMap returns a full map representation of the error, including the parsed
// message, all additional data (safe and unsafe), and recursively embedded
// original errors. Do NOT send this to external telemetry if PII may be present.
//
// The reserved keys "message", "stackTrace", "originalError", and
// "originalErrors" are produced by the error itself. Keys with these names in
// the data supplied through [WithSafeData] or [WithAdditionalData] are ignored.
//
// Wrapped errors are followed to a depth of 64 and at most 1000 are visited;
// deeper or further causes (which only occur if an error wraps itself) are
// omitted.
func (e Error) AsMap() map[string]any {
	budget := maxMapNodes
	return e.asMap(0, &budget)
}

func (e Error) asMap(depth int, budget *int) map[string]any {
	e.parse()
	temporaryMap := map[string]any{}
	maps.Copy(temporaryMap, e.safeData)
	maps.Copy(temporaryMap, e.additionalData)
	dropReservedKeys(temporaryMap)
	temporaryMap["message"] = e.parsedMessage

	if len(e.stack) > 0 {
		temporaryMap["stackTrace"] = e.StackTrace()
	}

	if depth >= maxMapDepth || spendNode(budget) {
		return temporaryMap
	}

	if len(e.originals) == 1 {
		temporaryMap["originalError"] = e.originals[0].Error()
		if embeddedError, ok := nerrors.AsType[*Error](e.originals[0]); ok && embeddedError != nil {
			temporaryMap["originalError"] = embeddedError.asMap(depth+1, budget)
		}
	} else if len(e.originals) > 1 {
		var errs []any
		for _, orig := range e.originals {
			if embeddedError, ok := nerrors.AsType[*Error](orig); ok && embeddedError != nil {
				errs = append(errs, embeddedError.asMap(depth+1, budget))
			} else {
				errs = append(errs, orig.Error())
			}
		}
		temporaryMap["originalErrors"] = errs
	}

	return temporaryMap
}

// SafeMap returns a redacted map representation containing only the message
// template (with format placeholders stripped), safe data, the stack trace, and
// the safe representation of wrapped library errors. PII-bearing fields
// (additionalData, parsed arguments) are excluded. Safe to send to Sentry,
// Datadog, or any external telemetry system.
//
// Wrapped errors that are not library errors are omitted: their text carries
// no safety classification. Use [Error.AsMap] for internal logs to see them.
//
// If [WithSafeParsedMessage] was provided, the "message" field contains the
// safe-formatted message. Otherwise, printf verbs are stripped to produce a
// clean, human-readable string (e.g., "user %s not found" becomes "user not found").
//
// The reserved keys "message", "stackTrace", "originalError", and
// "originalErrors" are produced by the error itself. Keys with these names in
// the data supplied through [WithSafeData] are ignored.
//
// The stack trace contains file paths and function names of the build machine
// unless the binary is built with -trimpath.
//
// Wrapped errors are followed to a depth of 64 and at most 1000 are visited;
// deeper or further causes (which only occur if an error wraps itself) are
// omitted.
func (e Error) SafeMap() map[string]any {
	budget := maxMapNodes
	return e.safeMap(0, &budget)
}

// spendNode consumes one unit of the visit budget and reports whether the
// budget is now exhausted, in which case the caller must stop descending.
func spendNode(budget *int) bool {
	*budget--
	return *budget <= 0
}

func (e Error) safeMap(depth int, budget *int) map[string]any {
	e.parse()
	msg := sanitizeTemplate(e.message)
	if e.safeMessage != "" {
		msg = e.safeMessage
	}
	safeMap := map[string]any{}
	maps.Copy(safeMap, e.safeData)
	dropReservedKeys(safeMap)
	safeMap["message"] = msg

	if len(e.stack) > 0 {
		safeMap["stackTrace"] = e.StackTrace()
	}

	if depth >= maxMapDepth || spendNode(budget) {
		return safeMap
	}

	var embedded []any
	for _, orig := range e.originals {
		if embeddedError, ok := nerrors.AsType[*Error](orig); ok && embeddedError != nil {
			embedded = append(embedded, embeddedError.safeMap(depth+1, budget))
		}
	}
	switch {
	case len(e.originals) == 1 && len(embedded) == 1:
		safeMap["originalError"] = embedded[0]
	case len(e.originals) > 1 && len(embedded) > 0:
		safeMap["originalErrors"] = embedded
	}

	return safeMap
}

// reservedKeys are produced by the error itself in AsMap and SafeMap.
var reservedKeys = []string{"message", "stackTrace", "originalError", "originalErrors"}

// dropReservedKeys removes the keys the error produces itself, so that user
// supplied data can neither shadow them nor fake their presence.
func dropReservedKeys(m map[string]any) {
	for _, key := range reservedKeys {
		delete(m, key)
	}
}

// StackTrace returns the captured call stack as a slice of human-readable
// "function file:line" strings. It returns nil if no stack was captured.
func (e Error) StackTrace() []string {
	if len(e.stack) == 0 {
		return nil
	}
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
// message template and collapses any resulting double spaces. An escaped
// percent sign ("%%") becomes a single literal percent sign.
func sanitizeTemplate(msg string) string {
	stripped := fmtVerbRegex.ReplaceAllStringFunc(msg, func(verb string) string {
		if verb == "%%" {
			return "%"
		}
		return ""
	})
	collapsed := multiSpace.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(collapsed)
}

var (
	fmtVerbRegex = regexp.MustCompile(`%%|%[+\-#0 ]*[0-9]*\.?[0-9]*[a-zA-Z]`)
	multiSpace   = regexp.MustCompile(`\s{2,}`)
)

// MarshalJSON implements [encoding/json.Marshaler] by marshalling the map
// representation returned by [Error.AsMap].
func (e Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.AsMap())
}
