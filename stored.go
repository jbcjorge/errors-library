package errors

// LateParse creates a [StoredError] whose option function is deferred until
// the error message is rendered (i.e. when [Error.Error] is called).
func LateParse(name ErrorType, opt ErrorOption) StoredError {
	return StoredError{
		lateParse: true,
		errorFunc: opt,
		name:      name,
	}
}

// ImmediateParse creates a [StoredError] whose option function is executed
// immediately when added to an [Error] via [New] or [Error.AddOptions].
func ImmediateParse(name ErrorType, opt ErrorOption) StoredError {
	return StoredError{
		lateParse: false,
		errorFunc: opt,
		name:      name,
	}
}
