package errors

// StoredError pairs an [ErrorOption] function with metadata controlling when
// it is applied. When lateParse is true the option is deferred until the error
// message is rendered; otherwise it runs immediately at construction time.
type StoredError struct {
	lateParse bool
	errorFunc ErrorOption
	name      ErrorType
}

// ErrorType identifies the kind of option stored in an [Error].
// Each type can appear at most once; adding a second option of the same type
// overwrites the first.
type ErrorType string

// Error is the central structured error type. It holds a message template,
// a lazily-rendered parsed message, an optional wrapped original error(s),
// arbitrary additional data (safe and unsafe), and a set of deferred options.
type Error struct {
	message        string
	parsedMessage  string
	safeMessage    string
	originals      []error
	additionalData map[string]any
	safeData       map[string]any
	options        map[ErrorType]ErrorOption
	stack          []uintptr
}

// ErrorOption is a function that mutates an [Error] to apply additional
// context such as a wrapped error, formatted message, or extra data.
type ErrorOption func(*Error)
