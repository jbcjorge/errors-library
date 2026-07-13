# errors

Structured, composable error handling.

## Features

- **Sentinel errors with deferred parsing** — declare error templates once, fill in parameters at the call site
- **Functional options** — `WithError`, `WithErrors`, `WithParsedMessage`, `WithAdditionalData`, `WithSafeData`, `WithSafeParsedMessage`
- **Stack traces** — automatically captured at error creation / parse site
- **Multi-cause errors** — wrap multiple errors, fully compatible with Go 1.20+ multi-unwrap
- **PII-safe redaction** — `SafeError()` and `SafeMap()` for telemetry-safe representations
- **`errors.Is` / `errors.As` compatible** — sentinel matching via message template identity
- **JSON marshaling** — `AsMap()` and `MarshalJSON()` for structured logging
- **Safe sentinels** — `Parse()` clones the error, never mutates the original
- **Stdlib superset** — re-exports `Is`, `As`, `Unwrap`, `Join` so a single import replaces `"errors"`

## Installation

```bash
go get github.com/jbcjorge/errors-library
```

## Usage

```go
// Declare sentinels at package level
var ErrNotFound = errors.New("resource %s not found")
var ErrValidation = errors.New("validation failed for field %s")

// Return with context — sentinel is never mutated
return ErrNotFound.Parse(errors.WithParsedMessage(resourceID))

// Wrap an underlying error
return ErrValidation.Parse(
    errors.WithParsedMessage(fieldName),
    errors.WithError(dbErr),
    errors.WithAdditionalData(map[string]any{"input": value}),
)

// Matching still works
errors.Is(err, ErrNotFound) // true
```

## Network portability (gRPC / cross-service errors)

This library is **not intended for sharing errors across service boundaries**.

If Service A needs to understand Service B's internal error types via `errors.Is`, that indicates tight coupling between services — a design smell in a microservice architecture. Services should communicate failures through well-defined gRPC status codes and API error contracts, not by leaking internal error types across the wire.

The recommended pattern at service boundaries:

1. Map internal errors to appropriate gRPC status codes (`NotFound`, `InvalidArgument`, etc.)
2. Log the full error with stack trace server-side
3. Return a clean, contract-defined error response to the caller

If a legitimate need for cross-service error propagation arises in the future (e.g., an internal gateway that must distinguish downstream failure modes), the library can be extended with proto-based serialization using gRPC error details. This is not currently implemented.

## Sentry integration

This library does not depend on `sentry-go`. Instead, use Sentry's `BeforeSend` hook to extract PII-safe data:

```go
sentry.Init(sentry.ClientOptions{
    BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
        if hint.OriginalException == nil {
            return event
        }
        var structuredErr *errors.Error
        if errors.As(hint.OriginalException, &structuredErr) {
            event.Message = structuredErr.SafeError()
            event.Extra = structuredErr.SafeMap()
        }
        return event
    },
})
```

This ensures:
- `event.Message` contains a PII-free error string (uses `WithSafeParsedMessage` values if provided, otherwise strips placeholders)
- `event.Extra` contains only safe metadata, stack trace, and sanitized nested errors
- No unsafe data (`WithAdditionalData`, interpolated PII) leaks to Sentry

## Development

### Prerequisites

- Go (version specified in `go.mod`)

### Running tests

Activate the local environment and run the test command:

```bash
source scripts/bootstrap.sh
testing
```

This runs:
- `go test` with coverage (reports written to `reports/`)
- `gocyclo` with a threshold of 15

### Manual checks

```bash
go test -count=1 ./...
go vet ./...
gofmt -s -w .
```

### Project layout

```
errors.go       # Core Error type, constructors, rendering
options.go      # Functional options (WithError, WithParsedMessage, etc.)
types.go        # Type definitions (StoredError, ErrorType, Error struct)
constants.go    # ErrorType constants
stdlib.go       # Re-exports of standard library functions (Is, As, Unwrap, Join)
stored.go       # LateParse / ImmediateParse constructors
doc.go          # Package documentation
```

## License

Apache 2.0. See [LICENSE](LICENSE).
