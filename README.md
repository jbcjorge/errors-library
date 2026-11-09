# errors

Structured, composable error handling.

## Features

- **Sentinel errors with deferred parsing**: declare error templates once, fill in parameters at the call site
- **Functional options**: `WithError`, `WithErrors`, `WithParsedMessage`, `WithAdditionalData`, `WithSafeData`, `WithSafeParsedMessage`, `WithHTTPStatus`
- **Stack traces**: automatically captured at error creation / parse site
- **Multi-cause errors**: wrap multiple errors, fully compatible with Go 1.20+ multi-unwrap
- **PII-safe redaction**: `SafeError()` and `SafeMap()` for telemetry-safe representations
- **HTTP responses**: associate a status code with `WithHTTPStatus`, write error responses with the `httperr` subpackage
- **`errors.Is` / `errors.As` compatible**: sentinel matching via message template identity
- **Structured logging, safe by default**: `MarshalJSON` encodes the telemetry-safe view, so `json.Marshal(err)` and a JSON `slog` handler never leak PII. `AsMap()` is the internal view; opt into it for JSON with `WithJSONOutputAsUnsafe` or `WithGeneralOutputAsUnsafe`
- **Safe sentinels**: `Parse()` clones the error, never mutates the original
- **Stdlib superset**: re-exports `Is`, `As`, `AsType`, `Unwrap`, `Join`, `ErrUnsupported` so a single import replaces `"errors"`

## Installation

```bash
go get github.com/jbcjorge/errors-library
```

## Usage

```go
// Declare sentinels at package level
var ErrNotFound = errors.New("resource %s not found")
var ErrValidation = errors.New("validation failed for field %s")

// Return with context; the sentinel is never mutated
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

### Rules worth knowing

- **Two views of an error, safe by default.** `SafeError()`, `SafeMap()` and `MarshalJSON()` are safe: they contain only what the author marked safe (`WithSafeData`, `WithSafeParsedMessage`) plus the safe view of wrapped library errors. So `json.Marshal(err)`, and a JSON `slog` handler, are safe to use anywhere. `Error()` and `AsMap()` are the internal view: they include `WithAdditionalData`, interpolated arguments, the text of wrapped errors, and the stack trace, and may contain PII. Use them for internal logs only. To make the JSON output use the internal view, choose it explicitly with `WithJSONOutputAsUnsafe` (only `MarshalJSON`) or `WithGeneralOutputAsUnsafe` (every implicit output); pass the option to `Parse` to scope it to one occurrence.
- **The library trusts the author.** It never inspects values. Anything passed to `WithSafeData` or `WithSafeParsedMessage` is treated as PII-free.
- **Stack traces carry build paths.** Frames contain file paths and function names of the build machine unless the binary is built with `-trimpath`.
- **Declare with `New`, derive with `Parse`.** `Parse` works on a copy and is safe for concurrent use of a shared sentinel. `AddOptions` mutates its receiver and must not be called on a sentinel that other goroutines use. Values placed in data maps are shared with the copies, so do not mutate maps or slices after passing them in.
- **Message templates use `fmt` verbs.** Write a literal percent sign as `%%`. If the arguments do not fit the template (missing, extra, or a wrong type), the template is used unchanged.
- **Reserved data keys.** `message`, `stackTrace`, `originalError` and `originalErrors` are produced by the error itself; data with these keys is ignored.
- **`New` panics on an empty message**, so a broken sentinel fails at program startup. Do not build messages from runtime values that may be empty, such as `New(err.Error())`.
- **Wrapped errors are followed to a depth of 64, and at most 1000 are visited** by `AsMap` and `SafeMap`. This only matters if an error wraps itself (directly or through others); the deeper causes are then omitted instead of crashing or hanging.

### Custom options

The built-in `With...` options cover most needs. To define your own, use `ImmediateParse`, most often to bundle built-in options that always travel together:

```go
// NotFound declares what every "not found" error of a service has in common.
func NotFound(resource string) errors.StoredError {
    return errors.ImmediateParse("notFound", func(e *errors.Error) {
        e.AddOptions(
            errors.WithHTTPStatus(http.StatusNotFound),
            errors.WithSafeData(map[string]any{"resource": resource}),
        )
    })
}

var ErrKeyMissing = errors.New("key not found", NotFound("key"))

return ErrKeyMissing.Parse(errors.WithError(cause)) // status 404 and resource "key" come along
```

The fields of `Error` are private, so a custom option works through its exported methods, above all `AddOptions`. Like every option, it can be passed to `New` to declare something once, or to `Parse` to apply it to a single occurrence.

`LateParse` is for the rare option that must be worked out every time the error is read, such as data computed lazily. It is how `WithParsedMessage` works. A late option runs on a private copy of the error each time you call `Error()`, `SafeError()`, `SafeMap()`, `AsMap()`, `MarshalJSON()` or `Original()`, so it never changes the error and is safe to read from many goroutines. It does not affect `HTTPStatus()`, `Originals()`, `Unwrap()` or `Is()`, and it must be deterministic and cheap. Late options run in order of their name; adding a second one with the same name replaces the first.

## Network portability (gRPC / cross-service errors)

This library is **not intended for sharing errors across service boundaries**.

If Service A needs to understand Service B's internal error types via `errors.Is`, that indicates tight coupling between services, which is a design smell in a microservice architecture. Services should communicate failures through well-defined gRPC status codes and API error contracts, not by leaking internal error types across the wire.

The recommended pattern at service boundaries:

1. Map internal errors to appropriate gRPC status codes (`NotFound`, `InvalidArgument`, etc.)
2. Log the full error with stack trace server-side
3. Return a clean, contract-defined error response to the caller

If a legitimate need for cross-service error propagation arises in the future (e.g., an internal gateway that must distinguish downstream failure modes), the library can be extended with proto-based serialization using gRPC error details. This is not currently implemented.

## HTTP error responses

The `httperr` subpackage writes a structured error as an HTTP response. The status
code travels with the error (set once at the sentinel), and the body is built from
the telemetry-safe `SafeMap()`, so data attached with `WithAdditionalData` never
reaches the client, and wrapped errors are never sent. A `Writer` is bound to one
`http.ResponseWriter` and renders through a pluggable `Renderer` (`JSON` is the
default; `ProblemJSON` for RFC 9457 is built in).

```go
import (
    "log/slog"
    "net/http"

    "github.com/jbcjorge/errors-library"
    "github.com/jbcjorge/errors-library/httperr"
)

// Declare the status alongside the sentinel; it is an invariant of the kind.
var ErrNotAuthorized = errors.New("not authorized",
    errors.WithHTTPStatus(http.StatusForbidden),
    errors.WithSafeData(map[string]any{"errorCode": "NOT_AUTHORIZED"}),
)

// Configure once; options are plain values.
var errOpts = []httperr.Option{httperr.WithRenderer(httperr.JSON(httperr.MessageKey("error")))}

func handler(w http.ResponseWriter, r *http.Request) {
    err := ErrNotAuthorized.Parse(
        errors.WithSafeData(map[string]any{"serial": "abc-123"}),      // safe -> body + telemetry
        errors.WithAdditionalData(map[string]any{"ip": r.RemoteAddr}), // unsafe -> logs only
    )

    writer := httperr.NewWriter(w, errOpts...)
    if werr := writer.Write(err); werr != nil {
        slog.Error("writing the error response failed", "err", werr)
        _ = writer.Fallback() // a bare 500 instead of an empty 200
    }
    if writer.Status() >= 500 {
        // log full detail server-side, e.g. slog.Error("...", "err", err)
    }
}
```

Body shape: the default `JSON` renderer writes the message under `"message"`
(override with `httperr.JSON(httperr.MessageKey("error"))`), plus any safe-data
keys. The stack trace is omitted unless you pass `httperr.WithStack()`, which
exposes build paths and is meant for development only. `<`, `>` and `&` are not
escaped.

`ProblemJSON` writes `type` (`about:blank`), `title`, `status` and `detail`. A
string `type`, `title` or `instance` in the error's safe data is used as is, which
is how an error declares a custom problem type; other safe data becomes extension
members.

Status resolution uses `httperr.StatusOf`, which walks the chain (outer first,
including wrapped causes) for the first `WithHTTPStatus`, inspects at most 10,000
errors, and falls back to `500`. A resolved status outside 400 to 599 makes
`Write` fail without sending anything.

Foreign errors (not an `*errors.Error`) carry no safety classification, so their
text is never forwarded. Map them to a native error with `httperr.WithConverter`,
or they are answered with `500` and a generic message (`httperr.WithFallbackMessage`).

`Write` renders into a buffer before sending, so a failure before the header is
sent leaves the response untouched and returns a sentinel you can match with
`errors.Is`: `ErrNothingToWrite`, `ErrEmptyMessage`, `ErrInvalidStatus`, `ErrRender`,
`ErrAlreadyWritten` (nothing sent), or `ErrWrite` (status sent, body failed). After
a failure that sent nothing, call `Fallback()`; otherwise the client receives an
empty 200. `Write` sets `Content-Type` and `X-Content-Type-Options: nosniff` and
removes any `Content-Length` set earlier.

Custom formats implement `Renderer` and plug in with `httperr.WithRenderer(...)`.
A renderer that panics propagates the panic; nothing has been sent at that point.

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
- `event.Extra` contains only safe metadata, the stack trace, and the safe view of wrapped library errors; wrapped errors that are not library errors are omitted
- No unsafe data (`WithAdditionalData`, interpolated PII) leaks to Sentry

The stack trace contains build paths unless the binary is built with `-trimpath`.

## Development

### Prerequisites

- Go (version specified in `go.mod`)

### Running the checks

```bash
make tools   # once: installs staticcheck, govulncheck, gosec, gocyclo, shadow, gocover-cobertura, gotestsum, gremlins
make check   # fmt check, vet, shadow, staticcheck, govulncheck, gosec, gitleaks, complexity, tests with -race, mutation (gremlins)
make fmt     # format the code
```

Keeping things up to date:

```bash
make update-deps               # upgrade direct dependencies, then build and test
make update-deps UPGRADE_GO=1  # also raise the go directive to the latest Go
make update-tools              # install the latest dev tools; re-pin them in the Makefile afterwards
```

The `go` directive in `go.mod` is the minimum Go version every consumer needs, so `update-deps` leaves it alone unless you pass `UPGRADE_GO=1`. If an upgraded dependency forces it up anyway, the target prints a warning with the old and new versions.

`make check` also needs `gitleaks` on the PATH. Test and coverage reports are
written to `reports/`.

### Project layout

```
errors.go       # Core Error type, constructors, rendering
options.go      # Functional options (WithError, WithParsedMessage, etc.)
types.go        # Type definitions (StoredError, ErrorType, Error struct)
constants.go    # ErrorType constants
stdlib.go       # Re-exports of standard library functions (Is, As, AsType, Unwrap, Join, ErrUnsupported)
stored.go       # ImmediateParse / LateParse: define your own options
doc.go          # Package documentation
errors_test.go  # Tests for the core package
extension_test.go # Tests that define custom options from outside the package
httperr/
  httperr.go    # Package documentation and the Write error sentinels
  writer.go     # Writer: Write, Fallback, payload resolution
  options.go    # Writer options (renderer, converter, fallback message, stack)
  renderer.go   # Renderer interface, JSON and ProblemJSON renderers
  status.go     # StatusOf: status resolution across an error tree
  *_test.go     # Tests (imports net/http)
```

## Behavior changes since v0.1.1

Read this before upgrading from v0.1.1:

- `New` panics when the message is empty or whitespace only.
- `MarshalJSON` now encodes the telemetry-safe view (`SafeMap`) instead of the internal one. `json.Marshal(err)` and JSON `slog` output no longer include `WithAdditionalData`, interpolated arguments, or wrapped-error text. Use `WithJSONOutputAsUnsafe` or `WithGeneralOutputAsUnsafe` to keep the old behavior for a given error.
- `AsMap()` now formats late-parsed messages (`WithParsedMessage`); before, it returned the raw template.
- `SafeMap()` omits wrapped errors that are not library errors. Before, it included their raw text, which could contain PII.
- The keys `message`, `stackTrace`, `originalError` and `originalErrors` are reserved. Data with these keys, passed through `WithSafeData` or `WithAdditionalData`, is ignored. Before, a data key named `message` replaced the real message.
- `SafeError()` and `SafeMap()` turn an escaped percent sign (`%%`) into a single `%`; before, part of the following word was removed (`100%% sure` became `100%ure`).
- `WithError(nil)` and `WithErrors(...)` ignore nil errors; before, a nil cause caused a panic later.
- Late-parsed options (`WithParsedMessage` and custom `LateParse` options) now run on a private copy of the error, in order of their name. Before, they ran on a copy that shared its data maps with the original, so a custom late option that wrote data raced under concurrent reads.
- `StackTrace()` returns nil for an error without a captured stack.
- New: `WithHTTPStatus`, `Error.HTTPStatus`, the `httperr` subpackage, and the `AsType` and `ErrUnsupported` re-exports.

## License

Apache 2.0. See [LICENSE](LICENSE).
