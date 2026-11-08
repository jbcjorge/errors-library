package httperr

import (
	"bytes"
	"encoding/json"
	nerrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jbcjorge/errors-library"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

// assertNothingSent checks that a failed Write left the response untouched.
func assertNothingSent(t *testing.T, rec *httptest.ResponseRecorder, wr *Writer) {
	t.Helper()
	assert.Equal(t, 0, rec.Body.Len())
	assert.Empty(t, rec.Header().Get("Content-Type"))
	assert.Equal(t, 0, wr.Status())
}

// --- happy path ---

func TestWrite_DefaultJSON(t *testing.T) {
	appErr := errors.New("not authorized",
		errors.WithHTTPStatus(http.StatusForbidden),
		errors.WithSafeData(map[string]any{"errorCode": "NOT_AUTHORIZED"}),
	)

	rec := httptest.NewRecorder()
	wr := NewWriter(rec)
	require.NoError(t, wr.Write(appErr))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, http.StatusForbidden, wr.Status())
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	body := decodeBody(t, rec)
	assert.Equal(t, "not authorized", body["message"])
	assert.Equal(t, "NOT_AUTHORIZED", body["errorCode"])
}

func TestWrite_MessageKey(t *testing.T) {
	appErr := errors.New("oops", errors.WithHTTPStatus(http.StatusTeapot))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(JSON(MessageKey("ErrorSays")))).Write(appErr))

	body := decodeBody(t, rec)
	assert.Equal(t, "oops", body["ErrorSays"])
	assert.NotContains(t, body, "message")
}

func TestWrite_EmptyMessageKeyKeepsDefault(t *testing.T) {
	appErr := errors.New("oops", errors.WithHTTPStatus(http.StatusBadRequest))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(JSON(MessageKey("")))).Write(appErr))

	assert.Equal(t, "oops", decodeBody(t, rec)["message"])
}

func TestWrite_MessageWinsOverDataCollision(t *testing.T) {
	// The JSON renderer must not let a safe data key replace the message when
	// the message key is configured to that very name.
	appErr := errors.New("real message",
		errors.WithHTTPStatus(http.StatusBadRequest),
		errors.WithSafeData(map[string]any{"error": "impostor", "code": "C1"}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(JSON(MessageKey("error")))).Write(appErr))

	body := decodeBody(t, rec)
	assert.Equal(t, "real message", body["error"])
	assert.Equal(t, "C1", body["code"])
}

func TestJSONRenderer_MessageBeatsDataKey(t *testing.T) {
	var buf bytes.Buffer
	err := JSON(MessageKey("error")).Render(&buf, Payload{
		Status:  http.StatusBadRequest,
		Message: "real",
		Data:    map[string]any{"error": "impostor"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"error":"real"}`, buf.String())
}

func TestWrite_StatusThroughWrapChain(t *testing.T) {
	sentinel := errors.New("forbidden", errors.WithHTTPStatus(http.StatusForbidden))
	wrapped := fmtWrap(sentinel.Parse(errors.WithError(nerrors.New("cause"))))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(wrapped))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "forbidden", decodeBody(t, rec)["message"])
}

func TestWrite_NativeWithoutStatusIs500(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(errors.New("no status")))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "no status", decodeBody(t, rec)["message"])
}

// --- safety ---

func TestWrite_UsesSafeMessage(t *testing.T) {
	appErr := errors.New("user %s not found",
		errors.WithHTTPStatus(http.StatusNotFound),
		errors.WithParsedMessage("john@example.com"),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(appErr))

	assert.Equal(t, "user not found", decodeBody(t, rec)["message"])
}

func TestWrite_ExcludesUnsafeData(t *testing.T) {
	appErr := errors.New("lookup failed",
		errors.WithHTTPStatus(http.StatusNotFound),
		errors.WithAdditionalData(map[string]any{"email": "john@example.com"}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(appErr))

	assert.NotContains(t, decodeBody(t, rec), "email")
}

func TestWrite_ExcludesStackAndOriginalsByDefault(t *testing.T) {
	appErr := errors.New("outer",
		errors.WithHTTPStatus(http.StatusBadGateway),
		errors.WithError(nerrors.New("inner")),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(appErr))

	body := decodeBody(t, rec)
	assert.NotContains(t, body, "stackTrace")
	assert.NotContains(t, body, "originalError")
}

func TestWrite_WithStack(t *testing.T) {
	appErr := errors.New("boom", errors.WithHTTPStatus(http.StatusBadGateway))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithStack()).Write(appErr))

	assert.Contains(t, decodeBody(t, rec), "stackTrace")
}

func TestWrite_NeverSendsASingleWrappedError(t *testing.T) {
	appErr := errors.New("outer",
		errors.WithHTTPStatus(http.StatusBadGateway),
		errors.WithError(errors.New("native cause secret")),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithStack()).Write(appErr))

	assert.NotContains(t, decodeBody(t, rec), "originalError")
	assert.NotContains(t, rec.Body.String(), "native cause secret")
}

func TestWrite_NeverSendsWrappedErrors(t *testing.T) {
	nativeCause := errors.New("native cause secret",
		errors.WithSafeData(map[string]any{"causeCode": "C"}))
	appErr := errors.New("outer",
		errors.WithHTTPStatus(http.StatusBadGateway),
		errors.WithErrors(nativeCause, nerrors.New("pg: password=hunter2")),
	)

	// Even with the stack enabled, no wrapped error content reaches the client.
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithStack()).Write(appErr))

	body := decodeBody(t, rec)
	assert.NotContains(t, body, "originalError")
	assert.NotContains(t, body, "originalErrors")
	assert.NotContains(t, rec.Body.String(), "hunter2")
	assert.NotContains(t, rec.Body.String(), "native cause secret")
	assert.NotContains(t, rec.Body.String(), "causeCode")
}

// --- foreign errors ---

func TestWrite_ForeignErrorUsesDefaultFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(nerrors.New("dial tcp 10.0.0.5:5432: connection refused")))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := decodeBody(t, rec)
	assert.Equal(t, DefaultFallbackMessage, body["message"])
	assert.NotContains(t, rec.Body.String(), "10.0.0.5") // foreign text never forwarded
}

func TestWrite_ForeignErrorCustomFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithFallbackMessage("something went wrong")).Write(nerrors.New("boom")))

	assert.Equal(t, "something went wrong", decodeBody(t, rec)["message"])
}

func TestWrite_ForeignErrorEmptyFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec, WithFallbackMessage(""))

	err := wr.Write(nerrors.New("boom"))
	assert.True(t, errors.Is(err, ErrEmptyMessage))
	assertNothingSent(t, rec, wr)
}

func TestWrite_ConverterHandlesForeignError(t *testing.T) {
	errNotFound := nerrors.New("sql: no rows in result set")
	errKeyNotFound := errors.New("key not found", errors.WithHTTPStatus(http.StatusNotFound))
	convert := func(err error) (*errors.Error, bool) {
		if nerrors.Is(err, errNotFound) {
			return errKeyNotFound, true
		}
		return nil, false
	}

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithConverter(convert)).Write(errNotFound))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "key not found", decodeBody(t, rec)["message"])
}

func TestWrite_ConverterDeclinesFallsBack(t *testing.T) {
	convert := func(error) (*errors.Error, bool) { return nil, false }

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithConverter(convert)).Write(nerrors.New("boom")))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, DefaultFallbackMessage, decodeBody(t, rec)["message"])
}

func TestWrite_ConverterReturningNilFallsBack(t *testing.T) {
	convert := func(error) (*errors.Error, bool) { return nil, true }

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithConverter(convert)).Write(nerrors.New("boom")))

	assert.Equal(t, DefaultFallbackMessage, decodeBody(t, rec)["message"])
}

// --- validation and failures ---

func TestWrite_NilError(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec)

	assert.True(t, errors.Is(wr.Write(nil), ErrNothingToWrite))
	assertNothingSent(t, rec, wr)
}

func TestWrite_EmptySafeMessage(t *testing.T) {
	// "%s" is a valid template but sanitizes to "" in the safe message.
	appErr := errors.New("%s", errors.WithParsedMessage("john@example.com"))

	rec := httptest.NewRecorder()
	wr := NewWriter(rec)

	assert.True(t, errors.Is(wr.Write(appErr), ErrEmptyMessage))
	assertNothingSent(t, rec, wr)
}

func TestWrite_InvalidStatus(t *testing.T) {
	for _, status := range []int{200, 302, 399, 600, 42} {
		rec := httptest.NewRecorder()
		wr := NewWriter(rec)

		err := wr.Write(errors.New("bad status", errors.WithHTTPStatus(status)))
		assert.True(t, errors.Is(err, ErrInvalidStatus), "status %d", status)
		assertNothingSent(t, rec, wr)
	}
}

func TestWrite_InvalidStatusMessageNamesStatus(t *testing.T) {
	err := NewWriter(httptest.NewRecorder()).Write(errors.New("bad", errors.WithHTTPStatus(200)))

	assert.Contains(t, err.Error(), "200")
}

func TestWrite_StatusBoundariesAccepted(t *testing.T) {
	for _, status := range []int{400, 599} {
		rec := httptest.NewRecorder()
		require.NoError(t, NewWriter(rec).Write(errors.New("edge", errors.WithHTTPStatus(status))))
		assert.Equal(t, status, rec.Code)
	}
}

func TestWrite_RenderFailureSendsNothing(t *testing.T) {
	appErr := errors.New("unencodable",
		errors.WithHTTPStatus(http.StatusBadRequest),
		errors.WithSafeData(map[string]any{"ch": make(chan int)}), // json cannot encode a channel
	)

	rec := httptest.NewRecorder()
	wr := NewWriter(rec)

	err := wr.Write(appErr)
	assert.True(t, errors.Is(err, ErrRender))
	assertNothingSent(t, rec, wr)

	// Nothing was sent, so the caller can still respond with the same writer.
	require.NoError(t, wr.Write(errors.New("fallback", errors.WithHTTPStatus(http.StatusInternalServerError))))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWrite_AlreadyWritten(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec)
	appErr := errors.New("first", errors.WithHTTPStatus(http.StatusBadRequest))

	require.NoError(t, wr.Write(appErr))
	assert.True(t, errors.Is(wr.Write(appErr), ErrAlreadyWritten))
	assert.Equal(t, http.StatusBadRequest, wr.Status())
}

// failingResponseWriter accepts headers but fails on body writes.
type failingResponseWriter struct {
	header http.Header
	status int
}

func (f *failingResponseWriter) Header() http.Header       { return f.header }
func (f *failingResponseWriter) WriteHeader(status int)    { f.status = status }
func (f *failingResponseWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWrite_BodyWriteFailure(t *testing.T) {
	fw := &failingResponseWriter{header: http.Header{}}
	wr := NewWriter(fw)

	err := wr.Write(errors.New("boom", errors.WithHTTPStatus(http.StatusBadGateway)))
	assert.True(t, errors.Is(err, ErrWrite))
	assert.True(t, nerrors.Is(err, io.ErrClosedPipe)) // cause is wrapped
	assert.Equal(t, http.StatusBadGateway, fw.status)
	assert.Equal(t, http.StatusBadGateway, wr.Status())
}

// --- renderers ---

func TestProblemJSON(t *testing.T) {
	appErr := errors.New("not authorized",
		errors.WithHTTPStatus(http.StatusForbidden),
		errors.WithSafeData(map[string]any{"errorCode": "NOT_AUTHORIZED", "status": "impostor"}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(ProblemJSON())).Write(appErr))

	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	body := decodeBody(t, rec)
	assert.Equal(t, "about:blank", body["type"])
	assert.Equal(t, "Forbidden", body["title"])
	assert.Equal(t, float64(http.StatusForbidden), body["status"]) // standard member wins
	assert.Equal(t, "not authorized", body["detail"])
	assert.Equal(t, "NOT_AUTHORIZED", body["errorCode"]) // extension member
	assert.NotContains(t, body, "message")
}

// plainTextRenderer is a consumer-defined renderer, proving the extension point.
type plainTextRenderer struct{}

func (plainTextRenderer) ContentType() string { return "text/plain; charset=utf-8" }

func (plainTextRenderer) Render(w io.Writer, p Payload) error {
	_, err := io.WriteString(w, http.StatusText(p.Status)+": "+p.Message)
	return err
}

func TestCustomRenderer(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(plainTextRenderer{})).
		Write(errors.New("key not found", errors.WithHTTPStatus(http.StatusNotFound))))

	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "Not Found: key not found", rec.Body.String())
}

func TestWithRendererNilKeepsDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(nil)).
		Write(errors.New("x", errors.WithHTTPStatus(http.StatusBadRequest))))

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestCustomRendererFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec, WithRenderer(failingRenderer{}))

	err := wr.Write(errors.New("x", errors.WithHTTPStatus(http.StatusBadRequest)))
	assert.True(t, errors.Is(err, ErrRender))
	assert.True(t, nerrors.Is(err, errRendererBroke)) // cause is wrapped
	assertNothingSent(t, rec, wr)
}

var errRendererBroke = nerrors.New("renderer broke")

type failingRenderer struct{}

func (failingRenderer) ContentType() string             { return "text/plain" }
func (failingRenderer) Render(io.Writer, Payload) error { return errRendererBroke }

// fmtWrap wraps err with a foreign fmt-style wrapper.
func fmtWrap(err error) error { return wrapped{err} }

type wrapped struct{ err error }

func (w wrapped) Error() string { return "context: " + w.err.Error() }
func (w wrapped) Unwrap() error { return w.err }

// --- headers ---

// snapshotWriter records the headers as they are at the moment WriteHeader is
// called, because headers set afterwards are not sent by a real server.
type snapshotWriter struct {
	header   http.Header
	atHeader http.Header
	status   int
	body     bytes.Buffer
}

func newSnapshotWriter() *snapshotWriter { return &snapshotWriter{header: http.Header{}} }

func (s *snapshotWriter) Header() http.Header { return s.header }
func (s *snapshotWriter) WriteHeader(status int) {
	s.status = status
	s.atHeader = s.header.Clone()
}
func (s *snapshotWriter) Write(b []byte) (int, error) { return s.body.Write(b) }

func TestWrite_HeadersAreSetBeforeWriteHeader(t *testing.T) {
	sw := newSnapshotWriter()

	require.NoError(t, NewWriter(sw).Write(errors.New("boom", errors.WithHTTPStatus(http.StatusBadRequest))))

	assert.Equal(t, "application/json", sw.atHeader.Get("Content-Type"))
	assert.Equal(t, "nosniff", sw.atHeader.Get("X-Content-Type-Options"))
}

func TestWrite_PresetContentLengthDoesNotBreakResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "3") // announced earlier, wrong for the error body
		_ = NewWriter(w).Write(errors.New("not found", errors.WithHTTPStatus(http.StatusNotFound)))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL) // #nosec G107 -- loopback test server
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Equal(t, "not found", body["message"])
}

// --- fallback ---

func TestFallback_AfterFailedWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec)

	require.True(t, errors.Is(wr.Write(nil), ErrNothingToWrite))
	require.NoError(t, wr.Fallback())

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, http.StatusInternalServerError, wr.Status())
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, DefaultFallbackMessage+"\n", rec.Body.String())
}

func TestFallback_CustomAndEmptyMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithFallbackMessage("try again later")).Fallback())
	assert.Equal(t, "try again later\n", rec.Body.String())

	rec = httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithFallbackMessage("  ")).Fallback())
	assert.Equal(t, http.StatusText(http.StatusInternalServerError)+"\n", rec.Body.String())
}

func TestFallback_AfterSuccessfulWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec)
	require.NoError(t, wr.Write(errors.New("first", errors.WithHTTPStatus(http.StatusBadRequest))))

	assert.True(t, errors.Is(wr.Fallback(), ErrAlreadyWritten))
	assert.Equal(t, http.StatusBadRequest, rec.Code) // first response untouched
}

func TestFallback_Twice(t *testing.T) {
	rec := httptest.NewRecorder()
	wr := NewWriter(rec)
	require.NoError(t, wr.Fallback())

	assert.True(t, errors.Is(wr.Fallback(), ErrAlreadyWritten))
	assert.True(t, errors.Is(wr.Write(errors.New("late")), ErrAlreadyWritten))
}

func TestFallback_BodyWriteFailure(t *testing.T) {
	fw := &failingResponseWriter{header: http.Header{}}

	err := NewWriter(fw).Fallback()
	assert.True(t, errors.Is(err, ErrWrite))
	assert.Equal(t, http.StatusInternalServerError, fw.status)
}

// --- encoding ---

func TestJSON_DoesNotEscapeHTMLCharacters(t *testing.T) {
	appErr := errors.New("need a<b & c>d", errors.WithHTTPStatus(http.StatusBadRequest))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(appErr))

	assert.Contains(t, rec.Body.String(), "need a<b & c>d")
	assert.NotContains(t, rec.Body.String(), `\u003c`)
}

func TestProblemJSON_DoesNotEscapeHTMLCharacters(t *testing.T) {
	appErr := errors.New("need a<b & c>d", errors.WithHTTPStatus(http.StatusBadRequest))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(ProblemJSON())).Write(appErr))

	assert.Contains(t, rec.Body.String(), "need a<b & c>d")
}

func TestProblemJSON_TypeTitleInstanceFromSafeData(t *testing.T) {
	appErr := errors.New("key expired",
		errors.WithHTTPStatus(http.StatusGone),
		errors.WithSafeData(map[string]any{
			"type":     "https://example.com/probs/expired",
			"title":    "Key expired",
			"instance": "/keys/abc-123",
			"detail":   "impostor",
		}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(ProblemJSON())).Write(appErr))

	body := decodeBody(t, rec)
	assert.Equal(t, "https://example.com/probs/expired", body["type"])
	assert.Equal(t, "Key expired", body["title"])
	assert.Equal(t, "/keys/abc-123", body["instance"])
	assert.Equal(t, "key expired", body["detail"]) // always from the error
	assert.Equal(t, float64(http.StatusGone), body["status"])
}

func TestProblemJSON_InvalidOverridesFallBackToDefaults(t *testing.T) {
	appErr := errors.New("gone",
		errors.WithHTTPStatus(http.StatusGone),
		errors.WithSafeData(map[string]any{"type": 42, "title": "", "instance": 7}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(ProblemJSON())).Write(appErr))

	body := decodeBody(t, rec)
	assert.Equal(t, "about:blank", body["type"])
	assert.Equal(t, "Gone", body["title"])
	assert.NotContains(t, body, "instance")
}

// --- unusual errors ---

func TestWrite_TypedNilErrorUsesFallback(t *testing.T) {
	var typedNil *errors.Error

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(typedNil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, DefaultFallbackMessage, decodeBody(t, rec)["message"])
}

func TestWrite_ErrorValueFromOriginal(t *testing.T) {
	// Original() on an error without causes returns an errors.Error value.
	value := errors.New("forbidden", errors.WithHTTPStatus(http.StatusForbidden)).Original()

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(value))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "forbidden", decodeBody(t, rec)["message"])
}

func TestWrite_LateParsedSafeMessageWithPercent(t *testing.T) {
	appErr := errors.New("quota %s at 100%% used",
		errors.WithHTTPStatus(http.StatusTooManyRequests),
		errors.WithParsedMessage("user-1"),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(appErr))

	assert.Equal(t, "quota at 100% used", decodeBody(t, rec)["message"])
}

func TestWrite_SelfWrappingErrorDoesNotCrashTheServer(t *testing.T) {
	// A native error that wraps itself used to overflow the stack in SafeMap,
	// which is a fatal error Go cannot recover from.
	self := errors.New("self referencing", errors.WithHTTPStatus(http.StatusInternalServerError))
	self.AddOptions(errors.WithError(self))

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec).Write(self))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "self referencing", decodeBody(t, rec)["message"])
}

func TestProblemJSON_StatusAndDetailCannotBeShadowedBySafeData(t *testing.T) {
	appErr := errors.New("real detail",
		errors.WithHTTPStatus(http.StatusConflict),
		errors.WithSafeData(map[string]any{"status": 200, "detail": "impostor"}),
	)

	rec := httptest.NewRecorder()
	require.NoError(t, NewWriter(rec, WithRenderer(ProblemJSON())).Write(appErr))

	body := decodeBody(t, rec)
	assert.Equal(t, float64(http.StatusConflict), body["status"])
	assert.Equal(t, "real detail", body["detail"])
}
