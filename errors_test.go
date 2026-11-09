package errors

import (
	"bytes"
	"encoding/json"
	nerrors "errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"
	"testing"
	"time"

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

func TestMarshalJSON_SafeByDefault(t *testing.T) {
	err := New("JSON error",
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}), // unsafe
		WithSafeData(map[string]any{"code": "E1"}),               // safe
	)

	jsonBytes, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(jsonBytes), "JSON error")
	assert.Contains(t, string(jsonBytes), "E1")
	assert.NotContains(t, string(jsonBytes), "123-45-6789") // unsafe data excluded by default
}

func TestMarshalJSON_EncodingJSONUsesSafeView(t *testing.T) {
	err := New("boom", WithAdditionalData(map[string]any{"ssn": "123-45-6789"}))

	// The implicit path: json.Marshal calls MarshalJSON.
	jsonBytes, marshalErr := json.Marshal(err)
	assert.NoError(t, marshalErr)
	assert.NotContains(t, string(jsonBytes), "123-45-6789")
}

func TestMarshalJSON_SlogJSONHandlerUsesSafeView(t *testing.T) {
	err := New("boom", WithAdditionalData(map[string]any{"ssn": "123-45-6789"}))

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Error("request failed", slog.Any("err", err))

	assert.NotContains(t, buf.String(), "123-45-6789")
	assert.Contains(t, buf.String(), "boom")
}

func TestWithJSONOutputAsUnsafe(t *testing.T) {
	err := New("boom",
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}),
		WithJSONOutputAsUnsafe(),
	)

	jsonBytes, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(jsonBytes), "123-45-6789") // explicit opt-out
}

func TestWithGeneralOutputAsUnsafe(t *testing.T) {
	err := New("boom",
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}),
		WithGeneralOutputAsUnsafe(),
	)

	jsonBytes, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(jsonBytes), "123-45-6789")
}

func TestUnsafeOutput_ScopedToOneParse(t *testing.T) {
	sentinel := New("boom", WithAdditionalData(map[string]any{"ssn": "123-45-6789"}))

	once := sentinel.Parse(WithJSONOutputAsUnsafe())
	onceBytes, _ := once.(*Error).MarshalJSON()
	assert.Contains(t, string(onceBytes), "123-45-6789") // this occurrence opted out

	sentinelBytes, _ := sentinel.MarshalJSON()
	assert.NotContains(t, string(sentinelBytes), "123-45-6789") // sentinel stays safe
}

func TestUnsafeOutput_SurvivesParse(t *testing.T) {
	// A sentinel that declared the unsafe knob keeps it through Parse.
	sentinel := New("boom",
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}),
		WithGeneralOutputAsUnsafe(),
	)
	parsed := sentinel.Parse()
	raw, _ := parsed.(*Error).MarshalJSON()
	assert.Contains(t, string(raw), "123-45-6789")
}

func TestUnsafeJSONOutput_SurvivesParse(t *testing.T) {
	sentinel := New("boom",
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}),
		WithJSONOutputAsUnsafe(),
	)
	parsed := sentinel.Parse()
	raw, _ := parsed.(*Error).MarshalJSON()
	assert.Contains(t, string(raw), "123-45-6789")
}

func TestUnsafeOutput_DoesNotAffectSafeMapOrSafeError(t *testing.T) {
	err := New("user %s",
		WithParsedMessage("secret"),
		WithAdditionalData(map[string]any{"ssn": "123-45-6789"}),
		WithGeneralOutputAsUnsafe(),
	)

	assert.NotContains(t, fmt.Sprint(err.SafeMap()), "123-45-6789")
	assert.NotContains(t, fmt.Sprint(err.SafeMap()), "secret")
	assert.Equal(t, "user", err.SafeError())
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
	var innerErr error = inner
	outer := fmt.Errorf("outer: %w", innerErr)

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

func TestWithHTTPStatus(t *testing.T) {
	err := New("not authorized", WithHTTPStatus(403))
	assert.Equal(t, 403, err.HTTPStatus())
}

func TestHTTPStatus_UnsetIsZero(t *testing.T) {
	err := New("no status")
	assert.Equal(t, 0, err.HTTPStatus())
}

func TestWithHTTPStatus_SurvivesParse(t *testing.T) {
	sentinel := New("forbidden", WithHTTPStatus(403))
	parsed := sentinel.Parse(WithError(nerrors.New("cause"))).(*Error)
	assert.Equal(t, 403, parsed.HTTPStatus())
	// Sentinel unchanged.
	assert.Equal(t, 403, sentinel.HTTPStatus())
}

func TestWithHTTPStatus_OverrideAtParse(t *testing.T) {
	sentinel := New("conflict", WithHTTPStatus(409))
	parsed := sentinel.Parse(WithHTTPStatus(400)).(*Error)
	assert.Equal(t, 400, parsed.HTTPStatus())
	assert.Equal(t, 409, sentinel.HTTPStatus()) // sentinel not mutated
}

func TestNew_PanicsOnEmptyMessage(t *testing.T) {
	assert.PanicsWithValue(t, "errors: New called with an empty message", func() { New("") })
}

func TestNew_PanicsOnWhitespaceMessage(t *testing.T) {
	assert.Panics(t, func() { New("   \t\n") })
}

func TestSafeMap_DataCannotShadowMessage(t *testing.T) {
	err := New("real message",
		WithSafeData(map[string]any{"message": "impostor"}),
	)
	assert.Equal(t, "real message", err.SafeMap()["message"])
}

func TestAsMap_DataCannotShadowMessage(t *testing.T) {
	err := New("real message",
		WithSafeData(map[string]any{"message": "safe impostor"}),
		WithAdditionalData(map[string]any{"message": "unsafe impostor"}),
	)
	assert.Equal(t, "real message", err.AsMap()["message"])
}

func TestSafeMap_DataCannotShadowReservedKeys(t *testing.T) {
	inner := New("inner")
	err := New("outer",
		WithError(inner),
		WithSafeData(map[string]any{
			"stackTrace":    "fake",
			"originalError": "fake",
		}),
	)
	safe := err.SafeMap()
	// Reserved keys reflect the real error, not the injected data.
	_, stackIsString := safe["stackTrace"].(string)
	assert.False(t, stackIsString)
	_, origIsMap := safe["originalError"].(map[string]any)
	assert.True(t, origIsMap)
}

// --- AsMap / MarshalJSON format late-parsed messages ---

func TestAsMap_FormatsLateParsedMessage(t *testing.T) {
	err := New("user %s", WithParsedMessage("bob"))
	assert.Equal(t, "user bob", err.AsMap()["message"])
}

func TestMarshalJSON_FormatsLateParsedSafeMessage(t *testing.T) {
	// Safe by default: the safe message uses WithSafeParsedMessage values.
	err := New("resource %s in %s",
		WithParsedMessage("secret-id", "prod"),
		WithSafeParsedMessage("resource", "prod"),
	)
	raw, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(raw), `"message":"resource resource in prod"`)
	assert.NotContains(t, string(raw), "secret-id")
}

func TestMarshalJSON_UnsafeFormatsLateParsedMessage(t *testing.T) {
	err := New("user %s", WithParsedMessage("bob"), WithJSONOutputAsUnsafe())
	raw, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(raw), `"message":"user bob"`)
}

// --- sentinel immutability, field by field ---

func TestParse_DoesNotMutateSentinelData(t *testing.T) {
	sentinel := New("sentinel %s",
		WithSafeData(map[string]any{"s": 1}),
		WithAdditionalData(map[string]any{"a": 1}),
		WithError(nerrors.New("cause")),
		WithHTTPStatus(409),
	)

	_ = sentinel.Parse(
		WithSafeData(map[string]any{"leak": "x"}),
		WithAdditionalData(map[string]any{"leak": "x"}),
		WithError(nerrors.New("extra")),
		WithHTTPStatus(400),
		WithParsedMessage("v"),
	)

	assert.NotContains(t, sentinel.SafeMap(), "leak")
	assert.NotContains(t, sentinel.AsMap(), "leak")
	assert.Len(t, sentinel.Originals(), 1)
	assert.Equal(t, 409, sentinel.HTTPStatus())
	assert.Equal(t, "sentinel %s", sentinel.Error())
}

func TestParse_ClonesDoNotShareOriginalsStorage(t *testing.T) {
	// Three single appends leave spare capacity in the backing array, which a
	// shared slice would let two clones overwrite.
	sentinel := New("sentinel",
		WithError(nerrors.New("a")),
		WithError(nerrors.New("b")),
		WithError(nerrors.New("c")),
	)
	first := nerrors.New("first")
	second := nerrors.New("second")

	p1 := sentinel.Parse(WithError(first)).(*Error)
	p2 := sentinel.Parse(WithError(second)).(*Error)

	assert.Same(t, first, p1.Originals()[3])
	assert.Same(t, second, p2.Originals()[3])
}

func newStackSentinel() *Error { return New("stack sentinel") }

func TestParse_RecapturesStackAtCallSite(t *testing.T) {
	sentinel := newStackSentinel()
	parsed := sentinel.Parse().(*Error)

	assert.Contains(t, sentinel.StackTrace()[0], "newStackSentinel")
	assert.Contains(t, parsed.StackTrace()[0], "TestParse_RecapturesStackAtCallSite")
}

// --- reserved keys ---

func TestSafeMap_ReservedKeysCannotBeFaked(t *testing.T) {
	// No cause was wrapped, so the error must not claim one.
	err := New("plain", WithSafeData(map[string]any{
		"originalError":  "fake",
		"originalErrors": "fake",
	}))
	safe := err.SafeMap()
	assert.NotContains(t, safe, "originalError")
	assert.NotContains(t, safe, "originalErrors")
}

func TestAsMap_ReservedKeysCannotBeOverwrittenByData(t *testing.T) {
	err := New("outer",
		WithError(New("inner")),
		WithSafeData(map[string]any{"message": "fake", "stackTrace": "fake"}),
		WithAdditionalData(map[string]any{"originalError": "fake", "stackTrace": "fake"}),
	)
	m := err.AsMap()
	assert.Equal(t, "outer", m["message"])
	_, stackIsList := m["stackTrace"].([]string)
	assert.True(t, stackIsList)
	_, causeIsMap := m["originalError"].(map[string]any)
	assert.True(t, causeIsMap)
}

// --- SafeMap never emits text of wrapped errors that are not library errors ---

func TestSafeMap_OmitsForeignOriginalError(t *testing.T) {
	err := New("lookup failed", WithError(nerrors.New("dial tcp 10.0.0.5:5432: password=hunter2")))
	safe := err.SafeMap()
	assert.NotContains(t, safe, "originalError")
	assert.NotContains(t, fmt.Sprint(safe), "hunter2")
}

func TestSafeMap_OmitsForeignOriginalErrors(t *testing.T) {
	err := New("batch failed", WithErrors(nerrors.New("ssn 123-45-6789"), nerrors.New("token=abc")))
	safe := err.SafeMap()
	assert.NotContains(t, safe, "originalErrors")
	assert.NotContains(t, fmt.Sprint(safe), "123-45-6789")
}

func TestSafeMap_KeepsOnlyNativeCausesInMixedList(t *testing.T) {
	native := New("native cause", WithSafeData(map[string]any{"code": "N1"}))
	err := New("batch failed", WithErrors(nerrors.New("secret"), native, nerrors.New("other secret")))

	causes, ok := err.SafeMap()["originalErrors"].([]any)
	assert.True(t, ok)
	assert.Len(t, causes, 1)
	assert.Equal(t, "N1", causes[0].(map[string]any)["code"])
	assert.NotContains(t, fmt.Sprint(err.SafeMap()), "secret")
}

func TestSafeMap_ForeignWrapperAroundNativeUsesNativeSafeView(t *testing.T) {
	native := New("native cause", WithSafeData(map[string]any{"code": "N1"}))
	var nativeErr error = native
	err := New("outer", WithError(fmt.Errorf("secret context: %w", nativeErr)))

	cause, ok := err.SafeMap()["originalError"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "N1", cause["code"])
	assert.NotContains(t, fmt.Sprint(err.SafeMap()), "secret context")
}

func TestAsMap_KeepsForeignOriginalText(t *testing.T) {
	// The internal view still shows everything, for logs.
	err := New("lookup failed", WithError(nerrors.New("dial tcp: refused")))
	assert.Equal(t, "dial tcp: refused", err.AsMap()["originalError"])
}

func TestAsMap_SingleVersusMultipleOriginals(t *testing.T) {
	// Exactly one original: singular key.
	single := New("outer", WithError(nerrors.New("only")))
	assert.Equal(t, "only", single.AsMap()["originalError"])
	assert.NotContains(t, single.AsMap(), "originalErrors")

	// More than one: plural key, singular absent. This pins the len > 1 boundary.
	multi := New("outer", WithErrors(nerrors.New("a"), nerrors.New("b")))
	errs, ok := multi.AsMap()["originalErrors"].([]any)
	assert.True(t, ok)
	assert.Len(t, errs, 2)
	assert.NotContains(t, multi.AsMap(), "originalError")
}

func TestSafeMap_SingleNativeCauseUsesSingularKey(t *testing.T) {
	err := New("outer", WithError(New("the one cause", WithSafeData(map[string]any{"c": "1"}))))
	safe := err.SafeMap()
	cause, ok := safe["originalError"].(map[string]any)
	assert.True(t, ok, "exactly one native cause must use the singular key")
	assert.Equal(t, "1", cause["c"])
	assert.NotContains(t, safe, "originalErrors")
}

func TestSafeMap_TwoNativeCausesUsePluralKey(t *testing.T) {
	err := New("outer",
		WithErrors(
			New("first", WithSafeData(map[string]any{"c": "1"})),
			New("second", WithSafeData(map[string]any{"c": "2"})),
		),
	)
	safe := err.SafeMap()
	causes, ok := safe["originalErrors"].([]any)
	assert.True(t, ok, "more than one native cause must use the plural key")
	assert.Len(t, causes, 2)
	assert.NotContains(t, safe, "originalError")
}

func TestSafeMap_SingleForeignCauseHasNoOriginalKey(t *testing.T) {
	// One original, but foreign: the singular branch must not fire.
	err := New("outer", WithError(nerrors.New("foreign")))
	safe := err.SafeMap()
	assert.NotContains(t, safe, "originalError")
	assert.NotContains(t, safe, "originalErrors")
}

func TestSafeMap_StackPresentVersusAbsent(t *testing.T) {
	withStack := New("has a stack")
	_, present := withStack.SafeMap()["stackTrace"]
	assert.True(t, present)

	// A zero-value Error has no captured stack: the len(stack) > 0 guard must hold.
	noStack := Error{message: "none", parsedMessage: "none", options: map[ErrorType]ErrorOption{}}
	_, absent := noStack.SafeMap()["stackTrace"]
	assert.False(t, absent)
}

// --- nil causes and stackless errors ---

func TestWithError_NilIsIgnored(t *testing.T) {
	err := New("x", WithError(nil))
	assert.Empty(t, err.Originals())
	assert.NotPanics(t, func() { err.SafeMap(); err.AsMap() })
}

func TestWithErrors_NilsAreIgnored(t *testing.T) {
	real := nerrors.New("real")
	err := New("x", WithErrors(nil, real, nil))
	assert.Equal(t, []error{real}, err.Originals())
	assert.NotPanics(t, func() { err.SafeMap(); err.AsMap() })
}

func TestStackTrace_NilWhenNoStackWasCaptured(t *testing.T) {
	assert.Nil(t, Error{}.StackTrace())
}

// --- message templates ---

func TestSafeError_EscapedPercentBecomesLiteral(t *testing.T) {
	err := New("quota %s at 100%% used", WithParsedMessage("user-1"))
	assert.Equal(t, "quota user-1 at 100% used", err.Error())
	assert.Equal(t, "quota at 100% used", err.SafeError())
}

func TestWithParsedMessage_ArgumentTextLookingLikeAFormatErrorIsKept(t *testing.T) {
	err := New("name: %s", WithParsedMessage("a%!b(c)"))
	assert.Equal(t, "name: a%!b(c)", err.Error())
}

func TestWithSafeParsedMessage_ArgumentTextLookingLikeAFormatErrorIsKept(t *testing.T) {
	err := New("name: %s", WithSafeParsedMessage("a%!b(c)"))
	assert.Equal(t, "name: a%!b(c)", err.SafeError())
}

func TestWithParsedMessage_MalformedFormattingKeepsTemplate(t *testing.T) {
	assert.Equal(t, "n=%d", New("n=%d", WithParsedMessage("not a number")).Error()) // wrong verb
	assert.Equal(t, "n=%d", New("n=%d", WithParsedMessage()).Error())               // missing
	assert.Equal(t, "x", New("x", WithParsedMessage("extra")).Error())              // extra
}

// --- stdlib re-exports ---

func TestStdlibReExports(t *testing.T) {
	cause := nerrors.New("cause")
	wrapped := fmt.Errorf("outer: %w", cause)

	assert.Equal(t, cause, Unwrap(wrapped))
	assert.True(t, Is(Join(nerrors.New("a"), cause), cause))
	assert.Equal(t, nerrors.ErrUnsupported, ErrUnsupported)

	native := New("native")
	var nativeErr error = native
	found, ok := AsType[*Error](fmt.Errorf("ctx: %w", nativeErr))
	assert.True(t, ok)
	assert.Same(t, native, found)

	_, ok = AsType[*Error](cause)
	assert.False(t, ok)
}

func TestAsMap_ReservedKeysCannotBeFakedWhenAbsent(t *testing.T) {
	// No cause and no stack: the error must not claim either through data.
	err := &Error{
		message:        "bare",
		parsedMessage:  "bare",
		additionalData: map[string]any{"originalError": "fake", "originalErrors": "fake", "stackTrace": "fake"},
		safeData:       map[string]any{},
		options:        map[ErrorType]ErrorOption{},
	}
	m := err.AsMap()
	assert.NotContains(t, m, "originalError")
	assert.NotContains(t, m, "originalErrors")
	assert.NotContains(t, m, "stackTrace")
}

// --- cyclic and deep error graphs ---

func TestSafeMap_CyclicGraphDoesNotOverflow(t *testing.T) {
	a := New("a")
	b := New("b", WithError(a))
	a.AddOptions(WithError(b)) // a -> b -> a

	assert.NotPanics(t, func() {
		safe := a.SafeMap()
		assert.Equal(t, "a", safe["message"])
	})
}

func TestAsMap_CyclicGraphDoesNotOverflow(t *testing.T) {
	a := New("a")
	b := New("b", WithError(a))
	a.AddOptions(WithError(b))

	assert.NotPanics(t, func() {
		assert.Equal(t, "a", a.AsMap()["message"])
	})
}

func TestSafeMap_SelfWrappingErrorDoesNotOverflow(t *testing.T) {
	self := New("self")
	self.AddOptions(WithError(self))

	assert.NotPanics(t, func() { self.SafeMap(); self.AsMap() })
}

// chainDepth counts the nested "originalError" levels of a map.
func chainDepth(m map[string]any) int {
	depth := 0
	for {
		next, ok := m["originalError"].(map[string]any)
		if !ok {
			return depth
		}
		depth++
		m = next
	}
}

func TestSafeMap_RealisticChainIsFullyReported(t *testing.T) {
	// A chain well under the bound must not be truncated.
	var err error = New("level 0")
	for i := 1; i <= 20; i++ {
		err = New(fmt.Sprintf("level %d", i), WithError(err))
	}
	top := err.(*Error)

	assert.Equal(t, 20, chainDepth(top.SafeMap()))
	assert.Equal(t, 20, chainDepth(top.AsMap()))
}

func TestSafeMap_ChainBeyondTheBoundIsTruncated(t *testing.T) {
	var err error = New("level 0")
	for i := 1; i <= maxMapDepth+10; i++ {
		err = New(fmt.Sprintf("level %d", i), WithError(err))
	}
	top := err.(*Error)

	assert.Equal(t, maxMapDepth, chainDepth(top.SafeMap()))
	assert.Equal(t, maxMapDepth, chainDepth(top.AsMap()))
}

// --- branching cycles need a total budget, not only a depth bound ---

// mapWithin fails the test instead of hanging when the map call does not return.
func mapWithin(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("map call did not return: the visit budget is not working")
	}
}

func TestSafeMap_BranchingSelfCycleTerminates(t *testing.T) {
	self := New("self")
	self.AddOptions(WithErrors(self, self)) // two causes, both itself: 2^64 paths

	mapWithin(t, func() { assert.Equal(t, "self", self.SafeMap()["message"]) })
}

func TestAsMap_BranchingSelfCycleTerminates(t *testing.T) {
	self := New("self")
	self.AddOptions(WithErrors(self, self))

	mapWithin(t, func() { assert.Equal(t, "self", self.AsMap()["message"]) })
}

func TestAsMap_MultiCauseCycleTerminates(t *testing.T) {
	a := New("a")
	b := New("b", WithErrors(a, nerrors.New("foreign")))
	a.AddOptions(WithErrors(b, nerrors.New("foreign")))

	mapWithin(t, func() { assert.Equal(t, "a", a.AsMap()["message"]) })
}

func TestAsMap_MultiCauseRendersNativeAsMapAndForeignAsText(t *testing.T) {
	native := New("native cause", WithSafeData(map[string]any{"code": "N1"}))
	err := New("outer", WithErrors(native, nerrors.New("foreign cause")))

	causes, ok := err.AsMap()["originalErrors"].([]any)
	assert.True(t, ok)
	assert.Len(t, causes, 2)

	nativeMap, isMap := causes[0].(map[string]any)
	assert.True(t, isMap, "a native cause must be rendered as its map")
	assert.Equal(t, "native cause", nativeMap["message"])
	assert.Equal(t, "N1", nativeMap["code"])
	assert.Equal(t, "foreign cause", causes[1], "a foreign cause is rendered as text in the internal view")
}

func TestSafeMap_WideTreeUnderTheBudgetIsFullyReported(t *testing.T) {
	causes := make([]error, 50)
	for i := range causes {
		causes[i] = New(fmt.Sprintf("cause %d", i))
	}
	err := New("outer", WithErrors(causes...))

	reported, ok := err.SafeMap()["originalErrors"].([]any)
	assert.True(t, ok)
	assert.Len(t, reported, 50)
}

// multiChainDepth counts the nested levels reached through the first entry of
// "originalErrors", the multi-cause shape.
func multiChainDepth(m map[string]any) int {
	depth := 0
	for {
		causes, ok := m["originalErrors"].([]any)
		if !ok || len(causes) == 0 {
			return depth
		}
		next, isMap := causes[0].(map[string]any)
		if !isMap {
			return depth
		}
		depth++
		m = next
	}
}

func TestAsMap_MultiCauseChainIsTruncatedAtTheDepthBound(t *testing.T) {
	// No cycle here, so only the depth bound (not the node budget) can stop it.
	var err error = New("level 0")
	for i := 1; i <= maxMapDepth+10; i++ {
		err = New(fmt.Sprintf("level %d", i), WithErrors(err, nerrors.New("sibling")))
	}
	top := err.(*Error)

	assert.Equal(t, maxMapDepth, multiChainDepth(top.AsMap()))
}

func TestSafeMap_MultiCauseChainIsTruncatedAtTheDepthBound(t *testing.T) {
	var err error = New("level 0")
	for i := 1; i <= maxMapDepth+10; i++ {
		err = New(fmt.Sprintf("level %d", i), WithErrors(err, New("sibling")))
	}
	top := err.(*Error)

	assert.Equal(t, maxMapDepth, multiChainDepth(top.SafeMap()))
}

// --- late-parsed options run on a private copy ---

// tenantLate is a late option the way a client writes one: it reaches the error
// only through the exported AddOptions, which writes into the error's data maps.
func tenantLate(name string) StoredError {
	return LateParse("clientTenant", func(e *Error) {
		e.AddOptions(WithSafeData(map[string]any{"tenant": name}))
	})
}

func TestLateOption_ConcurrentReadsDoNotRace(t *testing.T) {
	// Run with -race (make test does). Before late options ran on a private
	// copy, this wrote into the data maps shared by every reader.
	err := New("late %s", WithParsedMessage("x"), tenantLate("acme"))

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = err.Error()
			_ = err.SafeError()
			_ = err.SafeMap()
			_ = err.AsMap()
			_, _ = err.MarshalJSON()
		}()
	}
	wg.Wait()
}

func TestLateOption_DoesNotChangeTheDeclaredError(t *testing.T) {
	err := New("late", tenantLate("acme"))

	_ = err.SafeMap()
	_ = err.AsMap()
	_ = err.Error()

	assert.NotContains(t, err.safeData, "tenant", "reading must not write into the error")
	assert.Len(t, err.options, 1)
}

func TestLateOption_DataAppearsInEveryRenderedView(t *testing.T) {
	err := New("late", tenantLate("acme"))

	assert.Equal(t, "acme", err.SafeMap()["tenant"])
	assert.Equal(t, "acme", err.AsMap()["tenant"])
	raw, marshalErr := err.MarshalJSON()
	assert.NoError(t, marshalErr)
	assert.Contains(t, string(raw), `"tenant":"acme"`)
}

func TestLateOption_SurvivesParseAndStaysPerOccurrence(t *testing.T) {
	sentinel := New("late", tenantLate("acme"))

	assert.Equal(t, "acme", sentinel.Parse().(*Error).SafeMap()["tenant"])
	assert.NotContains(t, sentinel.safeData, "tenant")
}

func TestLateOptions_RunInAFixedOrder(t *testing.T) {
	appendTo := func(name ErrorType, suffix string) StoredError {
		return LateParse(name, func(e *Error) { e.parsedMessage += suffix })
	}
	// Registered out of order on purpose: they must run sorted by type name.
	err := New("base", appendTo("c", "C"), appendTo("a", "A"), appendTo("b", "B"))

	for range 50 {
		assert.Equal(t, "baseABC", err.Error())
	}
}

func TestLateOption_AddingALateOptionDoesNotRunItInTheSamePass(t *testing.T) {
	inner := LateParse("inner", func(e *Error) { e.parsedMessage += "-inner" })
	outer := LateParse("outer", func(e *Error) {
		e.parsedMessage += "-outer"
		e.AddOptions(inner)
	})
	err := New("base", outer)

	assert.Equal(t, "base-outer", err.Error())
	assert.Equal(t, "base-outer", err.Error(), "the added option must not accumulate across reads")
	assert.NotContains(t, err.options, ErrorType("inner"))
}

func TestLateOption_SameTypeReplacesTheEarlierOne(t *testing.T) {
	first := LateParse("same", func(e *Error) { e.parsedMessage += "-first" })
	second := LateParse("same", func(e *Error) { e.parsedMessage += "-second" })

	assert.Equal(t, "base-second", New("base", first, second).Error())
}

func TestOriginal_AppliesLateOptionsToTheReturnedValue(t *testing.T) {
	err := New("user %s", WithParsedMessage("bob"))

	original, ok := err.Original().(Error)
	assert.True(t, ok)
	assert.Equal(t, "user bob", original.parsedMessage)
}

func TestResolved_ZeroValueErrorIsSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		var zero Error
		_ = zero.resolved()
		_ = zero.SafeMap()
		_ = zero.AsMap()
	})
}
