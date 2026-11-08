package httperr

import (
	nerrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/jbcjorge/errors-library"

	"github.com/stretchr/testify/assert"
)

func TestStatusOf_DirectError(t *testing.T) {
	assert.Equal(t, 403, StatusOf(errors.New("forbidden", errors.WithHTTPStatus(403))))
}

func TestStatusOf_NilDefaults(t *testing.T) {
	assert.Equal(t, DefaultStatus, StatusOf(nil))
}

func TestStatusOf_NoStatusDefaults(t *testing.T) {
	assert.Equal(t, DefaultStatus, StatusOf(errors.New("no status set")))
}

func TestStatusOf_ForeignErrorDefaults(t *testing.T) {
	assert.Equal(t, DefaultStatus, StatusOf(nerrors.New("plain")))
}

func TestStatusOf_ThroughForeignWrapper(t *testing.T) {
	inner := errors.New("db forbidden", errors.WithHTTPStatus(403))
	var innerErr error = inner
	assert.Equal(t, 403, StatusOf(fmt.Errorf("service failed: %w", innerErr)))
}

func TestStatusOf_ThroughWithErrorCause(t *testing.T) {
	inner := errors.New("inner forbidden", errors.WithHTTPStatus(403))
	outer := errors.New("outer, no status", errors.WithError(inner))
	assert.Equal(t, 403, StatusOf(outer))
}

func TestStatusOf_OuterWins(t *testing.T) {
	inner := errors.New("inner", errors.WithHTTPStatus(403))
	outer := errors.New("outer", errors.WithHTTPStatus(409), errors.WithError(inner))
	assert.Equal(t, 409, StatusOf(outer))
}

func TestStatusOf_FirstOfMultipleCauses(t *testing.T) {
	noStatus := nerrors.New("plain cause")
	first := errors.New("first", errors.WithHTTPStatus(404))
	second := errors.New("second", errors.WithHTTPStatus(409))
	outer := errors.New("batch failed", errors.WithErrors(noStatus, first, second))
	assert.Equal(t, 404, StatusOf(outer))
}

func TestStatusOf_MultipleCausesNoneHaveStatus(t *testing.T) {
	outer := errors.New("batch failed", errors.WithErrors(nerrors.New("a"), nerrors.New("b")))
	assert.Equal(t, DefaultStatus, StatusOf(outer))
}

func TestStatusOf_ThroughJoin(t *testing.T) {
	joined := errors.Join(nerrors.New("plain"), errors.New("forbidden", errors.WithHTTPStatus(403)))
	assert.Equal(t, 403, StatusOf(joined))
}

func TestStatusOf_RuntimeOverrideBeatsSentinel(t *testing.T) {
	sentinel := errors.New("conflict", errors.WithHTTPStatus(409))
	assert.Equal(t, 400, StatusOf(sentinel.Parse(errors.WithHTTPStatus(400))))
	assert.Equal(t, 409, StatusOf(sentinel))
}

func TestStatusOf_TypedNilDoesNotPanic(t *testing.T) {
	var typedNil *errors.Error
	assert.Equal(t, DefaultStatus, StatusOf(typedNil))
}

func TestStatusOf_ErrorValue(t *testing.T) {
	value := errors.New("forbidden", errors.WithHTTPStatus(403)).Original()
	assert.Equal(t, 403, StatusOf(value))
}

// cyclic error trees must resolve to the default instead of hanging.

type loopErr struct{}

func (l *loopErr) Error() string { return "loop" }
func (l *loopErr) Unwrap() error { return l }

type multiLoopErr struct{}

func (l *multiLoopErr) Error() string   { return "multi loop" }
func (l *multiLoopErr) Unwrap() []error { return []error{l, l} }

type diamondErr struct{ children []error }

func (d *diamondErr) Error() string   { return "diamond" }
func (d *diamondErr) Unwrap() []error { return d.children }

// statusWithin fails the test instead of hanging when StatusOf does not return.
func statusWithin(t *testing.T, err error) int {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- StatusOf(err) }()
	select {
	case code := <-done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("StatusOf did not return")
		return 0
	}
}

func TestStatusOf_SingleUnwrapCycle(t *testing.T) {
	assert.Equal(t, DefaultStatus, statusWithin(t, &loopErr{}))
}

func TestStatusOf_MultiUnwrapCycle(t *testing.T) {
	assert.Equal(t, DefaultStatus, statusWithin(t, &multiLoopErr{}))
}

func TestStatusOf_ExponentialDiamond(t *testing.T) {
	var tree error = nerrors.New("leaf")
	for range 40 { // 2^40 paths without a visit budget
		tree = &diamondErr{children: []error{tree, tree}}
	}
	assert.Equal(t, DefaultStatus, statusWithin(t, tree))
}

func TestStatusOf_StatusFoundBeforeBudgetRunsOut(t *testing.T) {
	var tree error = errors.New("deep status", errors.WithHTTPStatus(418))
	for range 100 {
		tree = fmt.Errorf("layer: %w", tree)
	}
	assert.Equal(t, 418, statusWithin(t, tree))
}
