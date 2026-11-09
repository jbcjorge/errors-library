package errors_test

// These tests define custom options the way a user of the library does: from
// another package, through the exported API only. They guard the extension
// point documented on ImmediateParse and LateParse.

import (
	"net/http"
	"sync"
	"testing"

	errors "github.com/jbcjorge/errors-library"

	"github.com/stretchr/testify/assert"
)

// notFound bundles built-in options, as shown in the README.
func notFound(resource string) errors.StoredError {
	return errors.ImmediateParse("notFound", func(e *errors.Error) {
		e.AddOptions(
			errors.WithHTTPStatus(http.StatusNotFound),
			errors.WithSafeData(map[string]any{"resource": resource}),
		)
	})
}

// lazyTenant is a late option that attaches data when the error is read.
func lazyTenant(name string) errors.StoredError {
	return errors.LateParse("lazyTenant", func(e *errors.Error) {
		e.AddOptions(errors.WithSafeData(map[string]any{"tenant": name}))
	})
}

func TestCustomImmediateOption_BundlesBuiltInOptions(t *testing.T) {
	sentinel := errors.New("key not found", notFound("key"))

	assert.Equal(t, http.StatusNotFound, sentinel.HTTPStatus())
	assert.Equal(t, "key", sentinel.SafeMap()["resource"])
}

func TestCustomImmediateOption_AppliedToOneOccurrence(t *testing.T) {
	sentinel := errors.New("thing missing")

	occurrence, ok := sentinel.Parse(notFound("keys")).(*errors.Error)
	assert.True(t, ok)

	assert.Equal(t, http.StatusNotFound, occurrence.HTTPStatus())
	assert.Equal(t, "keys", occurrence.SafeMap()["resource"])
	assert.Zero(t, sentinel.HTTPStatus(), "the shared sentinel must stay untouched")
	assert.NotContains(t, sentinel.SafeMap(), "resource")
}

func TestCustomImmediateOption_ComposesWithBuiltIns(t *testing.T) {
	cause := assert.AnError
	sentinel := errors.New("%s missing", notFound("key"))

	occurrence := sentinel.Parse(errors.WithParsedMessage("key"), errors.WithError(cause))

	assert.Equal(t, "key missing", occurrence.Error())
	assert.ErrorIs(t, occurrence, cause)
	assert.True(t, errors.Is(occurrence, sentinel))
}

func TestCustomLateOption_ShowsUpInEveryRenderedView(t *testing.T) {
	occurrence := errors.New("late", lazyTenant("acme"))

	assert.Equal(t, "acme", occurrence.SafeMap()["tenant"])
	assert.Equal(t, "acme", occurrence.AsMap()["tenant"])
}

func TestCustomLateOption_LeavesTheErrorUntouched(t *testing.T) {
	sentinel := errors.New("late", lazyTenant("acme"))

	before := sentinel.SafeMap()
	_ = sentinel.Error()
	_ = sentinel.AsMap()

	assert.Equal(t, before["tenant"], sentinel.SafeMap()["tenant"])
	assert.Len(t, sentinel.Originals(), 0)
	assert.Zero(t, sentinel.HTTPStatus())
}

func TestCustomLateOption_SafeUnderConcurrentReads(t *testing.T) {
	// Run with -race (make test does).
	occurrence := errors.New("late %s", errors.WithParsedMessage("x"), lazyTenant("acme"))

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = occurrence.Error()
			_ = occurrence.SafeMap()
			_ = occurrence.AsMap()
		}()
	}
	wg.Wait()
}

func TestCustomLateOption_DoesNotAffectWhatIsNotRendered(t *testing.T) {
	// The documented limit of a late option: it shapes what is rendered, not
	// the status, the wrapped errors, or matching, which read the declared error.
	cause := assert.AnError
	late := errors.LateParse("lateEverything", func(e *errors.Error) {
		e.AddOptions(errors.WithHTTPStatus(http.StatusTeapot), errors.WithError(cause))
	})
	occurrence := errors.New("late", late)

	assert.Zero(t, occurrence.HTTPStatus())
	assert.Empty(t, occurrence.Originals())
	assert.Nil(t, occurrence.Unwrap())
	assert.False(t, errors.Is(occurrence, cause))
	assert.NotContains(t, occurrence.SafeMap(), "tenant")
}
