package errors

// ErrorType constants identify the built-in option kinds.
const (
	// TypeErrWrapped identifies the [WithError] option (wraps an original error).
	TypeErrWrapped ErrorType = "withError"
	// TypeErrMessage identifies the [WithParsedMessage] option (printf-style formatting).
	TypeErrMessage ErrorType = "parseData"
	// TypeErrSafeMessage identifies the [WithSafeParsedMessage] option (PII-free formatting).
	TypeErrSafeMessage ErrorType = "safeParseData"
	// TypeErrAdditionalData identifies the [WithAdditionalData] option (extra key-value pairs).
	TypeErrAdditionalData ErrorType = "additionalData"
	// TypeErrSafeData identifies the [WithSafeData] option (telemetry-safe key-value pairs).
	TypeErrSafeData ErrorType = "safeData"
	// TypeErrHTTPStatus identifies the [WithHTTPStatus] option (associated HTTP status code).
	TypeErrHTTPStatus ErrorType = "httpStatus"
	// TypeErrUnsafeJSON identifies the [WithJSONOutputAsUnsafe] option (internal view in MarshalJSON).
	TypeErrUnsafeJSON ErrorType = "unsafeJSON"
	// TypeErrUnsafeOutput identifies the [WithGeneralOutputAsUnsafe] option (internal view in every implicit output).
	TypeErrUnsafeOutput ErrorType = "unsafeOutput"
)
