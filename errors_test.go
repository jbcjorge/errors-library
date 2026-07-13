package errors

import (
	nerrors "errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalError(t *testing.T) {
	err := New("Mock error message")
	assert.NotNil(t, err)
	assert.Equal(t, "Mock error message", err.Error())
}

func TestUnwrap(t *testing.T) {
	op := "While testing"
	path := "/mocked/path"
	errString := "Mock error message"
	fsError := fs.PathError{
		Op:   op,
		Path: path,
		Err:  nerrors.New(errString),
	}
	err := New(errString, WithError(&fsError))
	assert.EqualError(t, err, errString)
	if wrappedError, ok := nerrors.AsType[*fs.PathError](err); !ok {
		assert.Failf(t, "error while checking unwrap capabilities", "can't unwrap the inner message")
	} else {
		assert.EqualError(t, wrappedError, fmt.Sprintf("%s %s: %s", op, path, err))
	}
}

func TestWithParsedMessage(t *testing.T) {
	err := New("Error: %s, code: %d", WithParsedMessage("test error", 500))
	assert.Equal(t, "Error: test error, code: 500", err.Error())
}

func TestWithAdditionalData(t *testing.T) {
	data := map[string]any{"key": "value", "count": 42}
	err := New("Error with data", WithAdditionalData(data))

	errMap := err.AsMap()
	assert.Equal(t, "value", errMap["key"])
	assert.Equal(t, 42, errMap["count"])
}

func TestOriginal_NoWrappedError(t *testing.T) {
	err := New("Simple error")
	original := err.Original()

	assert.Equal(t, *err, original)
}

func TestOriginal_WithWrappedError(t *testing.T) {
	innerErr := nerrors.New("inner error")
	err := New("Outer error", WithError(innerErr))

	original := err.Original()
	assert.Equal(t, innerErr, original)
}

func TestIs(t *testing.T) {
	err1 := New("Same message")
	err2 := New("Same message")
	err3 := New("Different message")

	assert.True(t, err1.Is(err2))
	assert.False(t, err1.Is(err3))
}

func TestIs_NotError(t *testing.T) {
	err1 := New("Structured error")
	err2 := nerrors.New("Standard error")

	assert.False(t, err1.Is(err2))
}

func TestAddNonValidParsedMessage(t *testing.T) {
	err := New("Base error")
	err.AddOptions(WithParsedMessage("formatted"))

	assert.Equal(t, "Base error", err.message)
	assert.Equal(t, "Base error", err.Error())
}

func TestParse(t *testing.T) {
	err := New("Error: %s", WithParsedMessage("test"))
	result := err.Parse()

	assert.Error(t, result)
	assert.Equal(t, "Error: test", result.Error())
}

func TestAsMap_WithNestedError(t *testing.T) {
	innerErr := New("Inner error")
	outerErr := New("Outer error", WithError(innerErr))

	errMap := outerErr.AsMap()
	assert.Equal(t, "Outer error", errMap["message"])

	originalMap, ok := errMap["originalError"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "Inner error", originalMap["message"])
}

func TestAsMap_WithStandardError(t *testing.T) {
	innerErr := nerrors.New("standard error")
	outerErr := New("Outer error", WithError(innerErr))

	errMap := outerErr.AsMap()
	assert.Equal(t, "standard error", errMap["originalError"])
}

func TestMarshalJSON(t *testing.T) {
	err := New("JSON error", WithAdditionalData(map[string]any{"code": 404}))

	jsonBytes, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(jsonBytes), "JSON error")
	assert.Contains(t, string(jsonBytes), "404")
}

func TestLateParse(t *testing.T) {
	stored := LateParse("test", func(e *Error) {
		e.parsedMessage = "late parsed"
	})

	assert.True(t, stored.lateParse)
	assert.Equal(t, ErrorType("test"), stored.name)
}

func TestImmediateParse(t *testing.T) {
	stored := ImmediateParse("test", func(e *Error) {
		e.parsedMessage = "immediate parsed"
	})

	assert.False(t, stored.lateParse)
	assert.Equal(t, ErrorType("test"), stored.name)
}

func TestParse_DoesNotMutateSentinel(t *testing.T) {
	sentinel := New("Error: %s")

	err1 := sentinel.Parse(WithParsedMessage("first"))
	err2 := sentinel.Parse(WithParsedMessage("second"))

	assert.Equal(t, "Error: first", err1.Error())
	assert.Equal(t, "Error: second", err2.Error())
	// Sentinel must remain unparsed
	assert.Equal(t, "Error: %s", sentinel.Error())
	// Is-matching still works
	assert.True(t, Is(err1, sentinel))
	assert.True(t, Is(err2, sentinel))
}

func TestAs_DirectMatch(t *testing.T) {
	err := New("some error", WithAdditionalData(map[string]any{"code": 404}))
	var target *Error
	assert.True(t, As(err, &target))
	assert.Equal(t, "some error", target.Error())
}

func TestAs_ThroughUnwrapChain(t *testing.T) {
	inner := New("inner error", WithAdditionalData(map[string]any{"code": 500}))
	outer := fmt.Errorf("outer: %w", inner)

	var target *Error
	assert.True(t, As(outer, &target))
	assert.Equal(t, "inner error", target.Error())
}

func TestAs_NestedErrors(t *testing.T) {
	inner := New("db failed")
	outer := New("query failed", WithError(inner))

	var target *Error
	assert.True(t, As(outer, &target))
	// As finds the first match — which is outer itself
	assert.Equal(t, "query failed", target.Error())
}

func TestStackTrace_CapturedOnNew(t *testing.T) {
	err := New("traced error")
	trace := err.StackTrace()
	assert.NotEmpty(t, trace)
	assert.Contains(t, trace[0], "TestStackTrace_CapturedOnNew")
}

func TestStackTrace_CapturedOnParse(t *testing.T) {
	sentinel := New("template %s")
	parsed := sentinel.Parse(WithParsedMessage("value")).(*Error)
	trace := parsed.StackTrace()
	assert.NotEmpty(t, trace)
	assert.Contains(t, trace[0], "TestStackTrace_CapturedOnParse")
}

func TestStackTrace_IncludedInAsMap(t *testing.T) {
	err := New("map error")
	m := err.AsMap()
	_, hasStack := m["stackTrace"]
	assert.True(t, hasStack)
}

func TestSafeMap_ExcludesUnsafeData(t *testing.T) {
	err := New("user %s not found",
		WithParsedMessage("john@example.com"),
		WithAdditionalData(map[string]any{"email": "john@example.com", "ip": "192.168.1.1"}),
		WithSafeData(map[string]any{"errorCode": "USER_NOT_FOUND", "httpStatus": 404}),
	)

	safe := err.SafeMap()
	// Safe map uses the sanitized template (placeholders stripped)
	assert.Equal(t, "user not found", safe["message"])
	// Safe data is present
	assert.Equal(t, "USER_NOT_FOUND", safe["errorCode"])
	assert.Equal(t, 404, safe["httpStatus"])
	// Unsafe data is absent
	assert.Nil(t, safe["email"])
	assert.Nil(t, safe["ip"])
}

func TestSafeMap_IncludesStackTrace(t *testing.T) {
	err := New("error")
	safe := err.SafeMap()
	_, hasStack := safe["stackTrace"]
	assert.True(t, hasStack)
}

func TestSafeMap_NestedErrors(t *testing.T) {
	inner := New("db error",
		WithAdditionalData(map[string]any{"query": "SELECT * FROM users WHERE email='pii'"}),
		WithSafeData(map[string]any{"table": "users"}),
	)
	outer := New("service error", WithError(inner), WithSafeData(map[string]any{"op": "getUser"}))

	safe := outer.SafeMap()
	assert.Equal(t, "getUser", safe["op"])

	innerSafe, ok := safe["originalError"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "users", innerSafe["table"])
	assert.Nil(t, innerSafe["query"]) // PII excluded
}

func TestAsMap_IncludesBothSafeAndUnsafe(t *testing.T) {
	err := New("error",
		WithAdditionalData(map[string]any{"secret": "pii-value"}),
		WithSafeData(map[string]any{"code": 500}),
	)

	full := err.AsMap()
	assert.Equal(t, "pii-value", full["secret"])
	assert.Equal(t, 500, full["code"])
}

func TestMultipleOptions(t *testing.T) {
	innerErr := nerrors.New("inner")
	err := New(
		"Error: %s with code %d",
		WithError(innerErr),
		WithParsedMessage("test", 500),
		WithAdditionalData(map[string]any{"extra": "data"}),
	)

	assert.Equal(t, "Error: test with code 500", err.Error())
	assert.Equal(t, innerErr, err.Original())

	errMap := err.AsMap()
	assert.Equal(t, "data", errMap["extra"])
}

func TestWithErrors_MultipleCauses(t *testing.T) {
	err1 := nerrors.New("field 'name' required")
	err2 := nerrors.New("field 'email' invalid")
	err3 := nerrors.New("field 'age' must be positive")

	err := New("validation failed", WithErrors(err1, err2, err3))

	assert.True(t, Is(err, err1))
	assert.True(t, Is(err, err2))
	assert.True(t, Is(err, err3))
	assert.Equal(t, 3, len(err.Originals()))
}

func TestWithErrors_AsMapMultiple(t *testing.T) {
	err1 := New("db error")
	err2 := nerrors.New("timeout")

	err := New("batch failed", WithErrors(err1, err2))

	m := err.AsMap()
	errs, ok := m["originalErrors"].([]any)
	assert.True(t, ok)
	assert.Equal(t, 2, len(errs))
}

func TestWithError_SingleCause_BackwardCompat(t *testing.T) {
	inner := nerrors.New("cause")
	err := New("wrapper", WithError(inner))

	assert.Equal(t, inner, err.Original())
	m := err.AsMap()
	assert.Equal(t, "cause", m["originalError"])
}

func TestSafeError(t *testing.T) {
	err := New("user %s not found", WithParsedMessage("john@example.com"))

	assert.Equal(t, "user john@example.com not found", err.Error())
	assert.Equal(t, "user not found", err.SafeError())
}

func TestSafeError_WithSafeParsedMessage(t *testing.T) {
	err := New("resource %s not found in %s",
		WithParsedMessage("user-123", "production"),
		WithSafeParsedMessage("resource", "production"),
	)

	assert.Equal(t, "resource user-123 not found in production", err.Error())
	assert.Equal(t, "resource resource not found in production", err.SafeError())
}

func TestSafeMap_WithSafeParsedMessage(t *testing.T) {
	err := New("error %d in %s",
		WithParsedMessage(500, "payments-service"),
		WithSafeParsedMessage(500, "payments-service"),
	)

	safe := err.SafeMap()
	assert.Equal(t, "error 500 in payments-service", safe["message"])
}

func TestSafeError_FallsBackToStripped(t *testing.T) {
	// No WithSafeParsedMessage — should strip placeholders
	err := New("user %s at %s failed", WithParsedMessage("john@pii.com", "10.0.0.1"))

	assert.Equal(t, "user at failed", err.SafeError())
}

func TestSafeParsedMessage_FewerArgs(t *testing.T) {
	err := New("error %s in %s at %s",
		WithParsedMessage("secret", "prod", "10.0.0.1"),
		WithSafeParsedMessage("code", "service"),
	)

	assert.Equal(t, "error secret in prod at 10.0.0.1", err.Error())
	// SafeMessage is empty (validation failed), falls back to stripped
	assert.Equal(t, "error in at", err.SafeError())
}
