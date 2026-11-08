package httperr

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
)

// Payload is the validated, telemetry-safe content of an error response, as
// handed to a [Renderer]. The writer guarantees Status is within 400 to 599
// and Message is not empty.
type Payload struct {
	// Status is the HTTP status code that will be sent.
	Status int
	// Message is the safe error message.
	Message string
	// Data holds the safe data of the error, plus the stack trace when enabled.
	// It never contains the message. Renderers may read it but must not keep a
	// reference after Render returns.
	Data map[string]any
}

// Renderer encodes a [Payload] into a response body. Implement it to support a
// custom wire format and plug it in with [WithRenderer].
type Renderer interface {
	// ContentType is the value of the Content-Type response header.
	ContentType() string
	// Render writes the encoded payload to w. If it returns an error, nothing
	// is sent to the client.
	Render(w io.Writer, p Payload) error
}

// DefaultMessageKey is the JSON body key for the message when [JSON] is used
// without [MessageKey]. It matches the key used by [errors.Error.SafeMap].
const DefaultMessageKey = "message"

// JSONOption configures the [JSON] renderer.
type JSONOption func(*jsonRenderer)

// MessageKey sets the JSON body key under which the message is written.
// An empty key is ignored.
func MessageKey(key string) JSONOption {
	return func(r *jsonRenderer) {
		if key != "" {
			r.messageKey = key
		}
	}
}

// JSON returns the default renderer. It writes Data as a JSON object with the
// message under the configured key (default [DefaultMessageKey]). The message
// always wins over a Data key of the same name. The characters <, > and & are
// not escaped.
func JSON(opts ...JSONOption) Renderer {
	r := jsonRenderer{messageKey: DefaultMessageKey}
	for _, opt := range opts {
		opt(&r)
	}
	return r
}

type jsonRenderer struct {
	messageKey string
}

func (jsonRenderer) ContentType() string { return "application/json" }

func (r jsonRenderer) Render(w io.Writer, p Payload) error {
	body := make(map[string]any, len(p.Data)+1)
	maps.Copy(body, p.Data)
	body[r.messageKey] = p.Message
	return encodeJSON(w, body)
}

// ProblemJSON returns a renderer producing RFC 9457 problem details
// (application/problem+json).
//
// "status" and "detail" always come from the error. "type" defaults to
// "about:blank" and "title" to the standard status text, but a string value for
// "type", "title", or "instance" in the safe data of the error is used as is,
// which is how an error declares a custom problem type:
//
//	errors.WithSafeData(map[string]any{"type": "https://example.com/probs/expired"})
//
// Other safe data is added as extension members. The message key setting of
// the JSON renderer does not apply.
func ProblemJSON() Renderer { return problemRenderer{} }

type problemRenderer struct{}

func (problemRenderer) ContentType() string { return "application/problem+json" }

func (problemRenderer) Render(w io.Writer, p Payload) error {
	body := make(map[string]any, len(p.Data)+4)
	maps.Copy(body, p.Data)
	if s, ok := body["type"].(string); !ok || s == "" {
		body["type"] = "about:blank"
	}
	if s, ok := body["title"].(string); !ok || s == "" {
		body["title"] = http.StatusText(p.Status)
	}
	if _, ok := body["instance"].(string); !ok {
		delete(body, "instance")
	}
	body["status"] = p.Status
	body["detail"] = p.Message
	return encodeJSON(w, body)
}

// encodeJSON writes v as JSON without escaping <, > and &, which are
// legitimate characters in an API error message.
func encodeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
