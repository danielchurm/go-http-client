package go_http_client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// RetrySettings configures retry behavior with exponential backoff.
//
// Fields:
//   - MaxRetries: Maximum number of retry attempts (default: 3)
//   - InitialInterval: Initial backoff duration before first retry (default: 500ms)
//   - MaxInterval: Maximum backoff duration between retries (default: 60s)
//   - Multiplier: Exponential backoff multiplier, e.g., 2.0 doubles each time (default: 1.5)
//   - RetriableStatusCodes: HTTP status codes that trigger retries (default: [429, 502, 503, 504])
//
// Zero values use sensible defaults. See WithRetries for usage examples.
type RetrySettings struct {
	MaxRetries           int           // default 3
	InitialInterval      time.Duration // default 500ms
	MaxInterval          time.Duration // default 60s
	Multiplier           float64       // default 1.5
	RetriableStatusCodes []int         // default [429, 502, 503, 504]
}

func (rs RetrySettings) applyDefaults() RetrySettings {
	if rs.MaxRetries == 0 {
		rs.MaxRetries = 3
	}

	if rs.InitialInterval == 0 {
		rs.InitialInterval = backoff.DefaultInitialInterval
	}

	if rs.MaxInterval == 0 {
		rs.MaxInterval = backoff.DefaultMaxInterval
	}

	if rs.Multiplier == 0 {
		rs.Multiplier = backoff.DefaultMultiplier
	}

	if rs.RetriableStatusCodes == nil {
		rs.RetriableStatusCodes = defaultRetriableStatusCodes
	}

	return rs
}

var defaultRetriableStatusCodes = []int{429, 502, 503, 504}

type retryTransport struct {
	next                 http.RoundTripper
	maxRetries           int
	initialInterval      time.Duration
	maxInterval          time.Duration
	multiplier           float64
	retriableStatusCodes []int
}

func newRetryTransport(next http.RoundTripper, settings RetrySettings) *retryTransport {
	settings = settings.applyDefaults()

	return &retryTransport{
		next:                 next,
		maxRetries:           settings.MaxRetries,
		initialInterval:      settings.InitialInterval,
		maxInterval:          settings.MaxInterval,
		multiplier:           settings.Multiplier,
		retriableStatusCodes: settings.RetriableStatusCodes,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isIdempotent(req.Method) {
		return t.next.RoundTrip(req)
	}

	var (
		bodyBytes []byte
		err       error
	)

	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		closeErr := req.Body.Close()
		if closeErr != nil {
			return nil, closeErr
		}
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = t.initialInterval
	bo.MaxInterval = t.maxInterval
	bo.Multiplier = t.multiplier

	op := func() (*http.Response, error) {
		reqClone := req.Clone(req.Context())
		if bodyBytes != nil {
			reqClone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := t.next.RoundTrip(reqClone)

		if err != nil {
			return nil, err
		}

		if !t.shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if retryAfter := parseRetryAfter(resp.Header.Get("Retry-After")); retryAfter > 0 {
				_, copyErr := io.Copy(io.Discard, resp.Body)
				if copyErr != nil {
					return nil, copyErr
				}
				closeErr := resp.Body.Close()
				if closeErr != nil {
					return nil, closeErr
				}

				return nil, &backoff.RetryAfterError{
					Duration: retryAfter,
				}
			}
		}

		_, copyErr := io.Copy(io.Discard, resp.Body)
		if copyErr != nil {
			return nil, copyErr
		}
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return nil, closeErr
		}

		return nil, fmt.Errorf("retriable status code: %d", resp.StatusCode)
	}

	resp, err := backoff.Retry(
		req.Context(),
		op,
		backoff.WithBackOff(bo),
		backoff.WithMaxTries(uint(t.maxRetries)+1), // +1 because we need to include the initial attempt
		backoff.WithMaxElapsedTime(0),              // no limit, rely on context
	)

	if err != nil {
		return nil, err
	}

	return resp, nil

}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace,
		http.MethodPut, http.MethodDelete:
		return true
	case http.MethodPost, http.MethodPatch: // being specific here for clarity
		return false
	default:
		return false
	}
}

func (t *retryTransport) shouldRetry(statusCode int) bool {
	return slices.Contains(t.retriableStatusCodes, statusCode)
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}

	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil {
		if seconds > 0 && seconds <= 300 { // 5 minutes max
			return time.Duration(seconds) * time.Second
		}
		return 0
	}

	if t, err := http.ParseTime(header); err == nil {
		duration := time.Until(t)
		if duration > 0 && duration <= 5*time.Minute {
			return duration
		}
	}

	return 0
}

func validateRetrySettings(settings RetrySettings, clientTimeout time.Duration) error {
	settings = settings.applyDefaults()

	if settings.MaxRetries < 0 {
		return fmt.Errorf("max retries must be >= 0, got: %d", settings.MaxRetries)
	}

	if settings.InitialInterval <= 0 {
		return fmt.Errorf("initial interval must be > 0, got: %v", settings.InitialInterval)
	}

	if settings.MaxInterval < settings.InitialInterval {
		return fmt.Errorf("max interval (%v) must be >= initial interval (%v)", settings.MaxInterval, settings.InitialInterval)
	}

	// Multiplier of 1 means constant backoff, >1 means exponential. We don't allow <1 because that would mean the backoff gets shorter with each retry, which is generally not desirable.
	if settings.Multiplier < 1.0 {
		return fmt.Errorf("multiplier must be >= 1.0, got: %v", settings.Multiplier)
	}

	worstCaseRetryTime := time.Duration(0)
	interval := settings.InitialInterval

	for range settings.MaxRetries {
		if interval > settings.MaxInterval {
			interval = settings.MaxInterval
		}
		worstCaseRetryTime += interval
		interval = time.Duration(float64(interval) * settings.Multiplier)
	}

	if worstCaseRetryTime >= clientTimeout {
		return fmt.Errorf(
			"worst case retry backoff (%v) must be less than client timeout (%v). "+
				"reduce max retries/max interval or increase client timeout",
			worstCaseRetryTime,
			clientTimeout,
		)
	}

	return nil
}
