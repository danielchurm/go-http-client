package go_http_client

import (
	"context"
	"net/http"
	"testing"
)

type mockRoundTripper struct {
	capturedReq *http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedReq = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
	}, nil
}

func TestHeaderTransport(t *testing.T) {
	type contextKey string

	tests := []struct {
		name            string
		staticHeaders   map[string]string
		contextHeaders  map[string]any
		requestHeaders  map[string]string // headers already on the request
		contextValues   map[any]any       // Can be any type, not just strings
		expectedHeaders map[string]string
	}{
		{
			name: "static headers are added",
			staticHeaders: map[string]string{
				"X-API-Key":    "secret",
				"X-Service-ID": "my-service",
			},
			expectedHeaders: map[string]string{
				"X-API-Key":    "secret",
				"X-Service-ID": "my-service",
			},
		},
		{
			name: "context headers are added from context",
			contextHeaders: map[string]any{
				"X-Request-ID": contextKey("request-id"),
				"X-Trace-ID":   contextKey("trace-id"),
			},
			contextValues: map[any]any{
				contextKey("request-id"): "req-123",
				contextKey("trace-id"):   "trace-456",
			},
			expectedHeaders: map[string]string{
				"X-Request-ID": "req-123",
				"X-Trace-ID":   "trace-456",
			},
		},
		{
			name: "context headers are not added if not in context",
			contextHeaders: map[string]any{
				"X-Request-ID": contextKey("request-id"),
			},
			contextValues:   map[any]any{},
			expectedHeaders: map[string]string{
				// X-Request-ID should not be added
			},
		},
		{
			name: "context headers with non-string values are converted to strings",
			contextHeaders: map[string]any{
				"X-Request-ID": contextKey("request-id"),
			},
			contextValues: map[any]any{
				contextKey("request-id"): 12345, // int converted to "12345"
			},
			expectedHeaders: map[string]string{
				"X-Request-ID": "12345",
			},
		},
		{
			name: "existing request headers are not overwritten by static headers",
			staticHeaders: map[string]string{
				"X-API-Key": "new-secret",
			},
			requestHeaders: map[string]string{
				"X-API-Key": "existing-secret",
			},
			expectedHeaders: map[string]string{
				"X-API-Key": "existing-secret",
			},
		},
		{
			name: "existing request headers are not overwritten by context headers",
			contextHeaders: map[string]any{
				"X-Request-ID": contextKey("request-id"),
			},
			requestHeaders: map[string]string{
				"X-Request-ID": "existing-req-id",
			},
			contextValues: map[any]any{
				contextKey("request-id"): "new-req-id",
			},
			expectedHeaders: map[string]string{
				"X-Request-ID": "existing-req-id",
			},
		},
		{
			name: "static and context headers are combined",
			staticHeaders: map[string]string{
				"X-API-Key":    "secret",
				"X-Service-ID": "my-service",
			},
			contextHeaders: map[string]any{
				"X-Request-ID": contextKey("request-id"),
			},
			contextValues: map[any]any{
				contextKey("request-id"): "req-123",
			},
			expectedHeaders: map[string]string{
				"X-API-Key":    "secret",
				"X-Service-ID": "my-service",
				"X-Request-ID": "req-123",
			},
		},
		{
			name:            "no headers added when both are empty",
			staticHeaders:   map[string]string{},
			contextHeaders:  map[string]any{},
			expectedHeaders: map[string]string{},
		},
		{
			name: "empty string static header value is added",
			staticHeaders: map[string]string{
				"X-Empty-Header": "",
			},
			expectedHeaders: map[string]string{
				"X-Empty-Header": "",
			},
		},
		{
			name: "empty string context header value is added",
			contextHeaders: map[string]any{
				"X-Empty-Context": contextKey("empty-ctx"),
			},
			contextValues: map[any]any{
				contextKey("empty-ctx"): "",
			},
			expectedHeaders: map[string]string{
				"X-Empty-Context": "",
			},
		},
		{
			name: "multiple headers with special characters",
			staticHeaders: map[string]string{
				"X-Custom-Header-1": "value-with-dashes",
				"X-Custom-Header-2": "value_with_underscores",
				"X-Custom-Header-3": "value-with-numbers-123",
			},
			expectedHeaders: map[string]string{
				"X-Custom-Header-1": "value-with-dashes",
				"X-Custom-Header-2": "value_with_underscores",
				"X-Custom-Header-3": "value-with-numbers-123",
			},
		},
		{
			name: "nil value in context is not added",
			contextHeaders: map[string]any{
				"X-Nil-Header": contextKey("nil-ctx"),
			},
			contextValues: map[any]any{
				contextKey("nil-ctx"): nil,
			},
			expectedHeaders: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			for key, value := range tt.contextValues {
				ctx = context.WithValue(ctx, key, value)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			for k, v := range tt.requestHeaders {
				req.Header.Set(k, v)
			}

			mock := &mockRoundTripper{}
			transport := newHeaderTransport(mock, HeaderSettings{
				StaticHeaders:  tt.staticHeaders,
				ContextHeaders: tt.contextHeaders,
			})

			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}
			_ = resp.Body.Close()

			if mock.capturedReq == nil {
				t.Fatal("request was not passed to next transport")
			}

			for expectedHeader, expectedValue := range tt.expectedHeaders {
				actualValue := mock.capturedReq.Header.Get(expectedHeader)
				if actualValue != expectedValue {
					t.Errorf("header %q: expected %q, got %q", expectedHeader, expectedValue, actualValue)
				}
			}

			// check we didn't add unexpected headers beyond what we expected and what was original
			for headerName := range mock.capturedReq.Header {
				found := false
				for expectedHeader := range tt.expectedHeaders {
					if http.CanonicalHeaderKey(expectedHeader) == http.CanonicalHeaderKey(headerName) {
						found = true
						break
					}
				}

				// check if this header was in the original request
				if !found {
					for origHeader := range tt.requestHeaders {
						if http.CanonicalHeaderKey(origHeader) == http.CanonicalHeaderKey(headerName) {
							found = true
							break
						}
					}
				}

				if !found {
					t.Errorf("unexpected header %q: %q", headerName, mock.capturedReq.Header.Get(headerName))
				}
			}
		})
	}
}
