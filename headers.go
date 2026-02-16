package go_http_client

import (
	"fmt"
	"net/http"
)

// HeaderSettings defines the configuration for adding headers to requests.
//
// ContextHeaders is a map of header name to context key. If the context contains a value for the key, it will be added as a header.
// All context values are converted to strings using fmt.Sprint (since all HTTP headers are strings).
// StaticHeaders is a map of header name to header value. These headers will be added to every request.
type HeaderSettings struct {
	ContextHeaders map[string]any
	StaticHeaders  map[string]string
}

type headerTransport struct {
	next     http.RoundTripper
	settings HeaderSettings
}

func newHeaderTransport(
	next http.RoundTripper,
	settings HeaderSettings,

) *headerTransport {
	return &headerTransport{
		next:     next,
		settings: settings,
	}
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqClone := req.Clone(req.Context())

	for k, v := range t.settings.StaticHeaders {
		if reqClone.Header.Get(k) == "" {
			reqClone.Header.Set(k, v)
		}
	}

	for header, ctxKey := range t.settings.ContextHeaders {
		if value := req.Context().Value(ctxKey); value != nil {
			// Check if header already exists in request
			if reqClone.Header.Get(header) != "" {
				continue
			}

			reqClone.Header.Set(header, fmt.Sprint(value))
		}
	}

	return t.next.RoundTrip(reqClone)
}
