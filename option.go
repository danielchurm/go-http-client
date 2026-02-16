package go_http_client

// Option is a functional option for configuring the HTTPClient.
// Provided options:
//   - WithRetries: Enable automatic retries with exponential backoff
//   - WithHeaders: Configure static and context-based headers
//   - WithCircuitBreakers: Configure circuit breakers for endpoints
//   - WithCircuitBreaker: Configure a single circuit breaker for a client
//   - WithConnectionPool: Configure connection pooling settings
//   - WithoutNewRelic: Disable New Relic instrumentation (not recommended)
type Option func(*config)

// WithoutNewRelic disables newrelic integration with the http client.
// Not recommended - only for use in tests and/or very specific scenarios
func WithoutNewRelic() Option {
	return func(cfg *config) {
		cfg.newRelicEnabled = false
	}
}

// WithCircuitBreakers configures multiple circuit breakers for an HTTP client.
// Define circuit breaker keys as constants and pass them in settings.
//
// If Name is omitted in gobreaker.Settings, the circuit breaker will be named after the key. (recommended)
//
// If OnStateChange is omitted in gobreaker.Settings, state changes will be logged with the default function:
//
//	func logCBStateChange(name string, from gobreaker.State, to gobreaker.State) {
//		log.WithFields(logrus.Fields{
//			"circuit_breaker": name,
//			"from_state":      from.String(),
//			"to_state":        to.String(),
//		}).Error("circuit breaker changed state")
//	}
//
// Example:
//
//	const BreakerPianoCollectorInfo CircuitBreakerKey = "piano:collector-info"
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithCircuitBreakers([]CircuitBreakerSettings{
//	        {
//	            Key: BreakerPianoCollectorInfo,
//	            Settings: gobreaker.Settings{
//	                Name:        string(BreakerPianoCollectorInfo),
//	                MaxRequests: 10,
//	                Interval:    60 * time.Second,
//	                Timeout:     30 * time.Second,
//	                ReadyToTrip: func(counts gobreaker.Counts) bool {
//	                    return counts.ConsecutiveFailures >= 5
//	                },
//	            },
//	            ShouldTrip: func(code int) bool { return code == 429 || code >= 500 },
//	        },
//	    }),
//	)
func WithCircuitBreakers(breakers []CircuitBreakerSettings) Option {
	return func(cfg *config) {
		for _, settings := range breakers {
			cfg.circuitBreakerSettings[settings.Key] = settings
		}
	}
}

// WithCircuitBreaker configures a single circuit breaker for an HTTP client.
// Define circuit breaker keys as constants and pass them in settings.
//
// Example:
//
//	const BreakerPianoCollectorInfo CircuitBreakerKey = "piano:collector-info"
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithCircuitBreaker(CircuitBreakerSettings{
//	            Key: BreakerPianoCollectorInfo,
//	            Settings: gobreaker.Settings{
//	                Name:        string(BreakerPianoCollectorInfo),
//	                MaxRequests: 10,
//	                Interval:    60 * time.Second,
//	                Timeout:     30 * time.Second,
//	                ReadyToTrip: func(counts gobreaker.Counts) bool {
//	                    return counts.ConsecutiveFailures >= 5
//	                },
//	            },
//	            ShouldTrip: func(code int) bool { return code == 429 || code >= 500 },
//	        },
//	    ),
//	)
func WithCircuitBreaker(settings CircuitBreakerSettings) Option {
	return func(cfg *config) {
		cfg.circuitBreakerSettings[settings.Key] = settings
	}
}

// WithRetries enables automatic retries with exponential backoff for transient failures.
// Retries are only attempted for idempotent HTTP methods (GET, PUT, DELETE, HEAD, OPTIONS, TRACE).
// POST and PATCH requests are never retried to avoid duplicate operations.
//
// Zero values in RetrySettings will use sensible defaults:
//   - MaxRetries: 3
//   - InitialInterval: 500ms
//   - MaxInterval: 60s
//   - Multiplier: 1.5
//   - RetriableStatusCodes: [429, 502, 503, 504]
//
// The retry mechanism respects Retry-After headers from servers (429, 503 responses).
// All retries are bound by the client timeout configured in NewClient.
//
// Example with defaults:
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithRetries(RetrySettings{}),
//	)
//
// Example with custom configuration:
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithRetries(RetrySettings{
//	        MaxRetries:           5,
//	        InitialInterval:      1 * time.Second,
//	        MaxInterval:          10 * time.Second,
//	        Multiplier:           2.0,
//	        RetriableStatusCodes: []int{503, 504}, // Only retry these
//	    }),
//	)
func WithRetries(settings RetrySettings) Option {
	return func(cfg *config) {
		cfg.retrySettings = &settings
	}
}

// WithHeaders configures static and context-based headers to be added to every request.
//
// Static headers are added to all requests. Context headers are extracted from the request
// context and added as HTTP headers, enabling automatic propagation of request IDs, trace IDs,
// and other contextual information across service boundaries.
//
// Headers are only added if not already present in the request, allowing per-request overrides.
//
// Example:
//
//	type contextKey string
//	const RequestIDKey contextKey = "request-id"
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithHeaders(HeaderSettings{
//	        ContextHeaders: map[string]any{
//	            "X-Request-ID": RequestIDKey,
//	        },
//	        StaticHeaders: map[string]string{
//	            "X-API-Key": "secret",
//	        },
//	    }),
//	)
//
//	// In your handler/middleware
//	ctx := context.WithValue(ctx, RequestIDKey, "req-12345")
//	resp, err := apiClient.GetUserWithResponse(ctx, "123")
//	// Request will include:
//	//   X-Request-ID: req-12345 (from context)
//	//   X-API-Key: secret (static)
func WithHeaders(settings HeaderSettings) Option {
	return func(cfg *config) {
		cfg.headerSettings = &settings
	}
}

// WithConnectionPool configures connection pooling settings for the HTTP client.
//
// These settings control how the client manages persistent connections to servers.
// Tuning these values can improve performance for high-throughput services or services
// making many concurrent requests to the same hosts.
//
// Example:
//
//	client, _ := NewClient(
//	    30 * time.Second,
//	    WithConnectionPool(PoolSettings{
//	        MaxIdleConns:        100,  // Total idle connections across all hosts
//	        MaxIdleConnsPerHost: 10,   // Idle connections per host
//	        IdleConnTimeout:     90 * time.Second, // How long idle connections stay open
//	    }),
//	)
func WithConnectionPool(settings PoolSettings) Option {
	return func(cfg *config) {
		cfg.poolSettings = &settings
	}
}
