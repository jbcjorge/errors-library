package errors

// StoredError is an option that can be added to an [Error]. The built-in ones
// are created by functions such as [WithError] and [WithSafeData]; your own are
// created with [ImmediateParse] and [LateParse]. When lateParse is true the
// option is deferred until the error is read; otherwise it runs immediately
// when it is added.
type StoredError struct {
	lateParse bool
	errorFunc ErrorOption
	name      ErrorType
}

// ErrorType identifies the kind of option stored in an [Error]. Late-parsed
// options are stored by type, so adding a second late-parsed option of the same
// type overwrites the first. Immediate options (wrapped errors, data, status)
// are applied when added: wrapped errors are appended and data is merged.
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
	httpStatus     int
	// unsafeJSON and unsafeOutput switch the implicit outputs (today only
	// MarshalJSON) from the safe view to the internal view. Both are off by
	// default: safe output is the default, the unsafe one is an explicit choice.
	unsafeJSON   bool
	unsafeOutput bool
}

// ErrorOption is a function that applies additional context to an [Error], such
// as a wrapped error, formatted message, or extra data. The fields of Error are
// not accessible outside this package, so an option written elsewhere works
// through the exported methods, above all [Error.AddOptions]. Turn one into an
// option with [ImmediateParse] (applied when added) or [LateParse] (applied on
// every read).
type ErrorOption func(*Error)
