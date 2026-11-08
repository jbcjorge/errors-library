package httperr

import "github.com/jbcjorge/errors-library"

// DefaultFallbackMessage is the message sent for errors that are not an
// [*errors.Error] and were not converted by [WithConverter].
const DefaultFallbackMessage = "internal error"

// Converter turns a foreign error (one that is not an [*errors.Error]) into a
// native one. It returns false when it does not handle err.
type Converter func(err error) (*errors.Error, bool)

// config holds the resolved writer options.
type config struct {
	renderer        Renderer
	converter       Converter
	fallbackMessage string
	includeStack    bool
}

func defaultConfig() config {
	return config{
		renderer:        JSON(),
		fallbackMessage: DefaultFallbackMessage,
	}
}

// Option configures a [Writer].
type Option func(*config)

// WithRenderer selects the renderer that encodes the payload. A nil renderer
// is ignored and the default ([JSON]) is kept. To change the JSON message key,
// pass a configured renderer: WithRenderer(JSON(MessageKey("error"))).
func WithRenderer(r Renderer) Option {
	return func(c *config) {
		if r != nil {
			c.renderer = r
		}
	}
}

// WithConverter registers a function that turns foreign errors into native
// ones, so they get a proper status and message instead of the fallback.
func WithConverter(fn Converter) Option {
	return func(c *config) { c.converter = fn }
}

// WithFallbackMessage overrides the message sent (with a 500) for foreign
// errors that no converter handled, and by [Writer.Fallback]. For [Writer.Write],
// an empty message makes foreign errors fail with [ErrEmptyMessage].
func WithFallbackMessage(msg string) Option {
	return func(c *config) { c.fallbackMessage = msg }
}

// WithStack includes the stack trace ("stackTrace") in the payload data. It is
// omitted by default.
//
// Stack frames carry source file paths and function names of the build
// machine unless the binary is built with -trimpath. Enable this for
// development only, never for responses to untrusted clients.
func WithStack() Option {
	return func(c *config) { c.includeStack = true }
}
