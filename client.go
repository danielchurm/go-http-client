package go_http_client

import (
	"fmt"
	"net/http"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// HTTPClient is a wrapper around http.Client that provides additional features such as:
//   - Automatic retries with exponential backoff for transient failures
//   - Configurable static and context-based headers
//   - Circuit breakers for specified endpoints
//   - New Relic instrumentation for monitoring and observability
type HTTPClient struct {
	*http.Client
	breakers map[CircuitBreakerKey]*circuitBreakerConfig
}

type config struct {
	timeout                time.Duration
	newRelicEnabled        bool
	retrySettings          *RetrySettings
	headerSettings         *HeaderSettings
	poolSettings           *PoolSettings
	circuitBreakerSettings map[CircuitBreakerKey]CircuitBreakerSettings
}

// NewClient creates a new HTTPClient with the specified timeout and optional configurations.
//
// The timeout parameter sets the maximum duration for the entire request, including all retries
// and backoff delays. It must be greater than 0.
//
// By default, NewClient integrates with New Relic for monitoring. Use WithoutNewRelic to disable.
//
// Available options:
//   - WithRetries: Enable automatic retries with exponential backoff
//   - WithHeaders: Configure static and context-based headers
//   - WithCircuitBreakers: Configure circuit breakers for endpoints
//   - WithCircuitBreaker: Configure a single circuit breaker for a client
//   - WithConnectionPool: Configure connection pooling settings
//   - WithoutNewRelic: Disable New Relic instrumentation (not recommended)
//
// Example usage:
//
//	client, err := NewClient(
//	    30 * time.Second,
//	    WithRetries(RetrySettings{
//	        MaxRetries:      3,
//	        InitialInterval: 500 * time.Millisecond,
//	    }),
//	    WithHeaders(HeaderSettings{
//	        ContextHeaders: map[string]any{
//	            "X-Request-ID": CtxKeyRequestID,
//	        },
//	        StaticHeaders: map[string]string{
//	            "X-API-Key": "secret",
//	        },
//	    }),
//	    WithCircuitBreaker(CircuitBreakerSettings{
//	        Key: BreakerUserService,
//	        Settings: gobreaker.Settings{
//	            Name:        string(BreakerUserService),
//	            MaxRequests: 10,
//	            Interval:    60 * time.Second,
//	            Timeout:     30 * time.Second,
//	            ReadyToTrip: func(counts gobreaker.Counts) bool {
//	                return counts.ConsecutiveFailures >= 5
//	            },
//	        },
//	        ShouldTrip: func(code int) bool { return code >= 500 },
//	    }),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewClient(timeout time.Duration, opts ...Option) (*HTTPClient, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than 0, got: %v", timeout)
	}

	cfg := &config{
		timeout:                timeout,
		newRelicEnabled:        true,
		circuitBreakerSettings: make(map[CircuitBreakerKey]CircuitBreakerSettings),
	}

	for _, o := range opts {
		o(cfg)
	}

	baseTransport := newBaseTransport(cfg.poolSettings)

	var transport http.RoundTripper = baseTransport

	if cfg.headerSettings != nil {
		transport = newHeaderTransport(transport, *cfg.headerSettings)
	}

	if cfg.newRelicEnabled {
		transport = newrelic.NewRoundTripper(transport)
	}

	if cfg.retrySettings != nil {
		if err := validateRetrySettings(*cfg.retrySettings, timeout); err != nil {
			return nil, fmt.Errorf("failed to validate retry settings: %w", err)
		}
		transport = newRetryTransport(transport, *cfg.retrySettings)
	}

	breakers := newCircuitBreakers(cfg.circuitBreakerSettings)

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.timeout,
	}

	return &HTTPClient{
		Client:   httpClient,
		breakers: breakers,
	}, nil
}
