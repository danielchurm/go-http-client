package go_http_client

import (
	"net/http"
	"testing"
	"time"
)

func TestNewBaseTransport_NilSettings(t *testing.T) {
	transport := newBaseTransport(nil)

	if transport == nil {
		t.Fatalf("expected transport, got nil")
	}

	// Should have cloned the default transport
	if transport.MaxIdleConns == 0 && transport.MaxIdleConnsPerHost == 0 {
		t.Errorf("expected transport to have default values")
	}
}

func TestNewBaseTransport_PoolSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings PoolSettings
		verify   func(*http.Transport, *testing.T)
	}{
		{
			name: "MaxIdleConns is applied",
			settings: PoolSettings{
				MaxIdleConns: 50,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxIdleConns != 50 {
					t.Errorf("expected MaxIdleConns 50, got %d", transport.MaxIdleConns)
				}
			},
		},
		{
			name: "MaxIdleConns zero is ignored",
			settings: PoolSettings{
				MaxIdleConns: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				// Should use default, not 0
				if transport.MaxIdleConns <= 0 {
					t.Errorf("expected default MaxIdleConns, got %d", transport.MaxIdleConns)
				}
			},
		},
		{
			name: "MaxIdleConnsPerHost is applied",
			settings: PoolSettings{
				MaxIdleConnsPerHost: 10,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxIdleConnsPerHost != 10 {
					t.Errorf("expected MaxIdleConnsPerHost 10, got %d", transport.MaxIdleConnsPerHost)
				}
			},
		},
		{
			name: "MaxConnsPerHost zero is applied (unlimited)",
			settings: PoolSettings{
				MaxConnsPerHost: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxConnsPerHost != 0 {
					t.Errorf("expected MaxConnsPerHost 0 (unlimited), got %d", transport.MaxConnsPerHost)
				}
			},
		},
		{
			name: "MaxConnsPerHost positive is applied",
			settings: PoolSettings{
				MaxConnsPerHost: 20,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxConnsPerHost != 20 {
					t.Errorf("expected MaxConnsPerHost 20, got %d", transport.MaxConnsPerHost)
				}
			},
		},
		{
			name: "IdleConnTimeout is applied",
			settings: PoolSettings{
				IdleConnTimeout: 5 * time.Second,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.IdleConnTimeout != 5*time.Second {
					t.Errorf("expected IdleConnTimeout 5s, got %v", transport.IdleConnTimeout)
				}
			},
		},
		{
			name: "IdleConnTimeout zero is ignored",
			settings: PoolSettings{
				IdleConnTimeout: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				// Should use default, not 0
				if transport.IdleConnTimeout == 0 {
					t.Errorf("expected default IdleConnTimeout, got 0")
				}
			},
		},
		{
			name: "ResponseHeaderTimeout is applied",
			settings: PoolSettings{
				ResponseHeaderTimeout: 10 * time.Second,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.ResponseHeaderTimeout != 10*time.Second {
					t.Errorf("expected ResponseHeaderTimeout 10s, got %v", transport.ResponseHeaderTimeout)
				}
			},
		},
		{
			name: "ResponseHeaderTimeout zero is ignored",
			settings: PoolSettings{
				ResponseHeaderTimeout: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.ResponseHeaderTimeout != 0 {
					t.Errorf("expected default ResponseHeaderTimeout, got %v", transport.ResponseHeaderTimeout)
				}
			},
		},
		{
			name: "TLSHandshakeTimeout is applied",
			settings: PoolSettings{
				TLSHandshakeTimeout: 15 * time.Second,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.TLSHandshakeTimeout != 15*time.Second {
					t.Errorf("expected TLSHandshakeTimeout 15s, got %v", transport.TLSHandshakeTimeout)
				}
			},
		},
		{
			name: "TLSHandshakeTimeout zero is ignored",
			settings: PoolSettings{
				TLSHandshakeTimeout: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				// Should use default, not 0
				if transport.TLSHandshakeTimeout == 0 {
					t.Errorf("expected default TLSHandshakeTimeout, got 0")
				}
			},
		},
		{
			name: "ExpectContinueTimeout is applied",
			settings: PoolSettings{
				ExpectContinueTimeout: 2 * time.Second,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.ExpectContinueTimeout != 2*time.Second {
					t.Errorf("expected ExpectContinueTimeout 2s, got %v", transport.ExpectContinueTimeout)
				}
			},
		},
		{
			name: "ExpectContinueTimeout zero is ignored",
			settings: PoolSettings{
				ExpectContinueTimeout: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				// Should use default, not 0
				if transport.ExpectContinueTimeout == 0 {
					t.Errorf("expected default ExpectContinueTimeout, got 0")
				}
			},
		},
		{
			name: "DisableKeepAlives true is applied",
			settings: PoolSettings{
				DisableKeepAlives: true,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if !transport.DisableKeepAlives {
					t.Errorf("expected DisableKeepAlives true, got false")
				}
			},
		},
		{
			name: "DisableKeepAlives false is applied",
			settings: PoolSettings{
				DisableKeepAlives: false,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.DisableKeepAlives {
					t.Errorf("expected DisableKeepAlives false, got true")
				}
			},
		},
		{
			name: "DisableCompression true is applied",
			settings: PoolSettings{
				DisableCompression: true,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if !transport.DisableCompression {
					t.Errorf("expected DisableCompression true, got false")
				}
			},
		},
		{
			name: "DisableCompression false is applied",
			settings: PoolSettings{
				DisableCompression: false,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.DisableCompression {
					t.Errorf("expected DisableCompression false, got true")
				}
			},
		},
		{
			name: "MaxResponseHeaderBytes is applied",
			settings: PoolSettings{
				MaxResponseHeaderBytes: 8192,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxResponseHeaderBytes != 8192 {
					t.Errorf("expected MaxResponseHeaderBytes 8192, got %d", transport.MaxResponseHeaderBytes)
				}
			},
		},
		{
			name: "MaxResponseHeaderBytes zero is ignored",
			settings: PoolSettings{
				MaxResponseHeaderBytes: 0,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				// Zero means default (10MB), so it should still be 0 in the struct
				if transport.MaxResponseHeaderBytes != 0 {
					t.Errorf("expected MaxResponseHeaderBytes 0, got %d", transport.MaxResponseHeaderBytes)
				}
			},
		},
		{
			name: "multiple settings applied together",
			settings: PoolSettings{
				MaxIdleConns:           50,
				MaxIdleConnsPerHost:    10,
				MaxConnsPerHost:        20,
				IdleConnTimeout:        5 * time.Second,
				ResponseHeaderTimeout:  10 * time.Second,
				TLSHandshakeTimeout:    15 * time.Second,
				ExpectContinueTimeout:  2 * time.Second,
				DisableKeepAlives:      true,
				DisableCompression:     true,
				MaxResponseHeaderBytes: 16384,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxIdleConns != 50 {
					t.Errorf("MaxIdleConns: expected 50, got %d", transport.MaxIdleConns)
				}
				if transport.MaxIdleConnsPerHost != 10 {
					t.Errorf("MaxIdleConnsPerHost: expected 10, got %d", transport.MaxIdleConnsPerHost)
				}
				if transport.MaxConnsPerHost != 20 {
					t.Errorf("MaxConnsPerHost: expected 20, got %d", transport.MaxConnsPerHost)
				}
				if transport.IdleConnTimeout != 5*time.Second {
					t.Errorf("IdleConnTimeout: expected 5s, got %v", transport.IdleConnTimeout)
				}
				if transport.ResponseHeaderTimeout != 10*time.Second {
					t.Errorf("ResponseHeaderTimeout: expected 10s, got %v", transport.ResponseHeaderTimeout)
				}
				if transport.TLSHandshakeTimeout != 15*time.Second {
					t.Errorf("TLSHandshakeTimeout: expected 15s, got %v", transport.TLSHandshakeTimeout)
				}
				if transport.ExpectContinueTimeout != 2*time.Second {
					t.Errorf("ExpectContinueTimeout: expected 2s, got %v", transport.ExpectContinueTimeout)
				}
				if !transport.DisableKeepAlives {
					t.Errorf("DisableKeepAlives: expected true, got false")
				}
				if !transport.DisableCompression {
					t.Errorf("DisableCompression: expected true, got false")
				}
				if transport.MaxResponseHeaderBytes != 16384 {
					t.Errorf("MaxResponseHeaderBytes: expected 16384, got %d", transport.MaxResponseHeaderBytes)
				}
			},
		},
		{
			name: "high concurrency settings",
			settings: PoolSettings{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 50,
				MaxConnsPerHost:     100,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.MaxIdleConns != 200 {
					t.Errorf("expected MaxIdleConns 200, got %d", transport.MaxIdleConns)
				}
				if transport.MaxIdleConnsPerHost != 50 {
					t.Errorf("expected MaxIdleConnsPerHost 50, got %d", transport.MaxIdleConnsPerHost)
				}
				if transport.MaxConnsPerHost != 100 {
					t.Errorf("expected MaxConnsPerHost 100, got %d", transport.MaxConnsPerHost)
				}
			},
		},
		{
			name: "aggressive timeout settings",
			settings: PoolSettings{
				ResponseHeaderTimeout: 2 * time.Second,
				TLSHandshakeTimeout:   3 * time.Second,
				ExpectContinueTimeout: 500 * time.Millisecond,
				IdleConnTimeout:       1 * time.Second,
			},
			verify: func(transport *http.Transport, t *testing.T) {
				if transport.ResponseHeaderTimeout != 2*time.Second {
					t.Errorf("ResponseHeaderTimeout: expected 2s, got %v", transport.ResponseHeaderTimeout)
				}
				if transport.TLSHandshakeTimeout != 3*time.Second {
					t.Errorf("TLSHandshakeTimeout: expected 3s, got %v", transport.TLSHandshakeTimeout)
				}
				if transport.ExpectContinueTimeout != 500*time.Millisecond {
					t.Errorf("ExpectContinueTimeout: expected 500ms, got %v", transport.ExpectContinueTimeout)
				}
				if transport.IdleConnTimeout != 1*time.Second {
					t.Errorf("IdleConnTimeout: expected 1s, got %v", transport.IdleConnTimeout)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := newBaseTransport(&tt.settings)

			if transport == nil {
				t.Fatalf("expected transport, got nil")
			}

			tt.verify(transport, t)
		})
	}
}

func TestNewBaseTransport_ReturnsNewTransport(t *testing.T) {
	settings := PoolSettings{
		MaxIdleConns: 25,
	}

	transport1 := newBaseTransport(&settings)
	transport2 := newBaseTransport(&settings)

	// Should return different instances
	if transport1 == transport2 {
		t.Errorf("expected different transport instances")
	}

	// But with same settings
	if transport1.MaxIdleConns != transport2.MaxIdleConns {
		t.Errorf("expected same settings, got different values")
	}
}

func TestNewBaseTransport_NegativeMaxConnsPerHost(t *testing.T) {
	// Negative MaxConnsPerHost should be applied (it's a valid setting)
	settings := PoolSettings{
		MaxConnsPerHost: -1,
	}

	transport := newBaseTransport(&settings)

	if transport.MaxConnsPerHost != -1 {
		t.Errorf("expected MaxConnsPerHost -1, got %d", transport.MaxConnsPerHost)
	}
}
