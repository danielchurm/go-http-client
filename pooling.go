package go_http_client

import (
	"net/http"
	"time"
)

// PoolSettings configures HTTP connection pooling and transport behavior.
//
// Connection pooling improves performance by reusing TCP connections across requests.
// Zero values use http.Transport defaults.
//
// Connection pool limits:
//   - MaxIdleConns: Maximum idle connections across all hosts (default: 100)
//   - MaxIdleConnsPerHost: Maximum idle connections per host (default: 2)
//   - MaxConnsPerHost: Maximum total connections per host, 0 = unlimited (default: 0)
//   - IdleConnTimeout: How long idle connections stay open (default: 90s)
//
// Fine-grained timeouts:
//   - ResponseHeaderTimeout: Maximum time to wait for response headers (default: 0, no timeout)
//   - TLSHandshakeTimeout: Maximum time for TLS handshake (default: 10s)
//   - ExpectContinueTimeout: Maximum time to wait for 100-continue response (default: 1s)
//
// Advanced settings:
//   - DisableKeepAlives: Disable HTTP keep-alives (default: false)
//   - DisableCompression: Disable gzip compression (default: false)
//   - MaxResponseHeaderBytes: Maximum response header size, 0 = 10MB (default: 0)
type PoolSettings struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration

	ResponseHeaderTimeout time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration

	DisableKeepAlives      bool
	DisableCompression     bool
	MaxResponseHeaderBytes int64
}

func newBaseTransport(settings *PoolSettings) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if settings != nil {
		if settings.MaxIdleConns > 0 {
			transport.MaxIdleConns = settings.MaxIdleConns
		}
		if settings.MaxIdleConnsPerHost > 0 {
			transport.MaxIdleConnsPerHost = settings.MaxIdleConnsPerHost
		}
		if settings.MaxConnsPerHost != 0 {
			transport.MaxConnsPerHost = settings.MaxConnsPerHost
		}
		if settings.IdleConnTimeout > 0 {
			transport.IdleConnTimeout = settings.IdleConnTimeout
		}
		if settings.ResponseHeaderTimeout > 0 {
			transport.ResponseHeaderTimeout = settings.ResponseHeaderTimeout
		}
		if settings.TLSHandshakeTimeout > 0 {
			transport.TLSHandshakeTimeout = settings.TLSHandshakeTimeout
		}
		if settings.ExpectContinueTimeout > 0 {
			transport.ExpectContinueTimeout = settings.ExpectContinueTimeout
		}
		transport.DisableKeepAlives = settings.DisableKeepAlives
		transport.DisableCompression = settings.DisableCompression
		if settings.MaxResponseHeaderBytes > 0 {
			transport.MaxResponseHeaderBytes = settings.MaxResponseHeaderBytes
		}
	}

	return transport
}
