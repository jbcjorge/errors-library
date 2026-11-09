package errors

// ImmediateParse creates a custom option that is applied right away, when it
// is added to an [Error] through [New], [Error.Parse] or [Error.AddOptions].
// This is the way to define your own reusable options.
//
// The function receives the error being built. Its fields are not accessible
// from outside this package, so a custom option does its work through the
// exported methods, above all [Error.AddOptions], by bundling the built-in
// options:
//
//	// NotFound declares what every "not found" error of a service has in common.
//	func NotFound(resource string) errors.StoredError {
//		return errors.ImmediateParse("notFound", func(e *errors.Error) {
//			e.AddOptions(
//				errors.WithHTTPStatus(http.StatusNotFound),
//				errors.WithSafeData(map[string]any{"resource": resource}),
//			)
//		})
//	}
//
//	var ErrKeyMissing = errors.New("key not found", NotFound("key"))
//	return ErrKeyMissing.Parse(errors.WithError(cause))
//
// The name is only a label for an immediate option; it does not need to be
// unique. As with every option, pass it to [Error.Parse] to apply it to one
// occurrence without touching a shared sentinel.
func ImmediateParse(name ErrorType, opt ErrorOption) StoredError {
	return StoredError{
		lateParse: false,
		errorFunc: opt,
		name:      name,
	}
}

// LateParse creates a custom option that is stored on the error and applied
// each time the error is read, instead of when it is added. The built-in
// [WithParsedMessage] and [WithSafeParsedMessage] work this way, which is how
// a sentinel can be declared first and filled in at the call site.
//
// Use it only when the value must be worked out at read time, for example
// data that is computed lazily. Otherwise prefer [ImmediateParse].
//
// The rules for a late option:
//
//   - It runs on a private copy of the error, every time the error is read
//     through [Error.Error], [Error.SafeError], [Error.SafeMap],
//     [Error.AsMap], [Error.MarshalJSON] or [Error.Original]. Whatever it
//     does is visible in that read only; the error itself is never changed,
//     so it is safe to read one error from many goroutines.
//   - It has no effect on what is not rendered: [Error.HTTPStatus],
//     [Error.Originals], [Error.Unwrap] and [Error.Is] read the error as it
//     was declared.
//   - It must be deterministic and cheap, because it runs on every read.
//   - Late options run in order of their name, and an option added by a late
//     option waits for the next read.
//   - The name identifies the option: adding a second late option with the
//     same name replaces the first. Do not reuse a built-in name (see the
//     TypeErr constants).
func LateParse(name ErrorType, opt ErrorOption) StoredError {
	return StoredError{
		lateParse: true,
		errorFunc: opt,
		name:      name,
	}
}
