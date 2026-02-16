package go_http_client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestIntegration_SimpleGetRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	client, err := NewClient(10*time.Second, WithoutNewRelic())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "success" {
		t.Errorf("expected body 'success', got %q", string(body))
	}
	_ = resp.Body.Close()
}

func TestIntegration_HeadersAreIncluded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithHeaders(HeaderSettings{
			StaticHeaders: map[string]string{
				"X-API-Key": "test-key",
			},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_ContextHeadersAreIncluded(t *testing.T) {
	type ctxKey string
	const requestIDKey ctxKey = "request-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID != "test-123" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithHeaders(HeaderSettings{
			ContextHeaders: map[string]any{
				"X-Request-ID": requestIDKey,
			},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.WithValue(context.Background(), requestIDKey, "test-123")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_RetryOnServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			// First call fails
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      2,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      1.5,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls (1 failure + 1 retry), got %d", callCount)
	}
}

func TestIntegration_RetryGetsExhausted(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Always fail
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      2,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      1.5,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)

	// When retries are exhausted with a retriable status code, the retry transport returns an error
	if err == nil {
		_ = resp.Body.Close()
		// If no error, verify we got the failed response
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", resp.StatusCode)
		}
	}

	// Should have tried 3 times (1 initial + 2 retries)
	if callCount != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", callCount)
	}
}

func TestIntegration_NonIdempotentMethodsNotRetried(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// POST always fails
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      2,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      1.5,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Should have only tried once (POST is not idempotent)
	if callCount != 1 {
		t.Errorf("expected 1 call (no retries for POST), got %d", callCount)
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestIntegration_CircuitBreakerTrips_ShouldTrip(t *testing.T) {
	const breakerKey CircuitBreakerKey = "test-service"

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Always fail with 500
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithCircuitBreaker(CircuitBreakerSettings{
			Key: breakerKey,
			Settings: gobreaker.Settings{
				Name:        breakerKey.String(),
				MaxRequests: 1,
				Interval:    1 * time.Second,
				Timeout:     1 * time.Second,
				ReadyToTrip: func(counts gobreaker.Counts) bool {
					// Trip after first failure
					return counts.ConsecutiveFailures >= 1
				},
			},
			ShouldTrip: func(statusCode int) bool {
				return statusCode >= 500
			},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	breaker := client.GetBreaker(breakerKey)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// First request should fail normally
	resp1, err := breaker.Execute(func() (*http.Response, error) {
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if client.ShouldTrip(breakerKey, res.StatusCode) {
			return res, fmt.Errorf("status code %d triggers circuit breaker", res.StatusCode)
		}
		return res, nil
	})
	if err == nil {
		t.Fatal("expected execute to fail but got nil error")
	}
	if resp1 == nil {
		t.Fatal("expected response to be non-nil even on failure for ShouldTrip")
	}

	_ = resp1.Body.Close()

	if resp1.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected first response to be 500, got %d", resp1.StatusCode)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call so far, got %d", callCount)
	}

	// Second request should trip the circuit breaker
	// The breaker should be in open state now
	resp2, err := breaker.Execute(func() (*http.Response, error) {
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if client.ShouldTrip(breakerKey, res.StatusCode) {
			return res, fmt.Errorf("status code %d triggers circuit breaker", res.StatusCode)
		}
		return res, nil
	})
	if err == nil {
		_ = resp2.Body.Close()
	}

	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected circuit breaker to be open, but it is %s", breaker.State())
	}

	// Circuit breaker prevents the second call
	// Note: gobreaker returns an error when the circuit is open
	if err == nil && resp2.StatusCode != http.StatusInternalServerError {
		t.Logf("second request completed with status %d", resp2.StatusCode)
	}
}

func TestIntegration_CircuitBreakerTrips_NoShouldTrip(t *testing.T) {
	const breakerKey CircuitBreakerKey = "test-service"

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithCircuitBreaker(CircuitBreakerSettings{
			Key: breakerKey,
			Settings: gobreaker.Settings{
				Name:        breakerKey.String(),
				MaxRequests: 1,
				Interval:    1 * time.Second,
				Timeout:     1 * time.Second,
				ReadyToTrip: func(counts gobreaker.Counts) bool {
					// Trip after first failure
					return counts.ConsecutiveFailures >= 1
				},
			},
			ShouldTrip: func(statusCode int) bool {
				return statusCode >= 500
			},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	breaker := client.GetBreaker(breakerKey)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// First request should fail normally
	resp1, err := breaker.Execute(func() (*http.Response, error) {
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusForbidden {
			return res, fmt.Errorf("status code %d triggers circuit breaker", res.StatusCode)
		}
		return res, nil
	})
	if err == nil {
		t.Fatal("expected execute to fail but got nil error")
	}
	if resp1 == nil {
		t.Fatal("expected response to be non-nil even on failure for ShouldTrip")
	}

	_ = resp1.Body.Close()

	if resp1.StatusCode != http.StatusForbidden {
		t.Errorf("expected first response to be 403, got %d", resp1.StatusCode)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call so far, got %d", callCount)
	}

	// Second request should trip the circuit breaker
	// The breaker should be in open state now
	resp2, err := breaker.Execute(func() (*http.Response, error) {
		_, _ = client.Do(req)
		return nil, nil
	})
	if err == nil {
		_ = resp2.Body.Close()
		t.Fatalf("expected cb to return error for blocked request")
	}

	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected circuit breaker to be open, but it is %s", breaker.State())
	}

	// Circuit breaker prevents the second call
	// Note: gobreaker returns an error when the circuit is open
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected error to be gobreaker.ErrOpenState, got %v", err)
	}
}

func TestIntegration_ExecuteWithBreakerHelper(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Always return 503 to trigger circuit breaker
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithCircuitBreaker(CircuitBreakerSettings{
			Key: testKey,
			Settings: gobreaker.Settings{
				Name:        string(testKey),
				MaxRequests: 1,
				Interval:    60 * time.Second,
				Timeout:     60 * time.Second,
				ReadyToTrip: func(counts gobreaker.Counts) bool {
					return counts.ConsecutiveFailures >= 1
				},
			},
			ShouldTrip: func(code int) bool {
				return code >= 500
			},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// First call should fail and trip the breaker
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return client.Do(req)
	})

	if !errors.Is(err, ErrBadResponse) {
		t.Errorf("expected ErrBadResponse, got %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should be blocked by open circuit breaker
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return client.Do(req2)
	})

	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("expected gobreaker.ErrOpenState, got %v", err)
	}

	// Should still be only 1 call (second was blocked)
	if callCount != 1 {
		t.Errorf("expected still 1 call (blocked by breaker), got %d", callCount)
	}

	breaker := client.GetBreaker(testKey)
	if breaker.State() != gobreaker.StateOpen {
		t.Errorf("expected circuit breaker to be open, got %s", breaker.State())
	}
}

func TestIntegration_RequestBodyIsPreservedOnRetry(t *testing.T) {
	callCount := 0
	receivedBodies := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))

		if callCount < 2 {
			// First call fails
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      1,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      1.5,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	const testBody = `{"key":"value"}`
	// Use PUT which is idempotent and can have a body, so retries will actually happen
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, server.URL, strings.NewReader(testBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if callCount != 2 {
		t.Errorf("expected 2 calls (1 failure + 1 retry), got %d", callCount)
	}

	if len(receivedBodies) != 2 {
		t.Errorf("expected 2 bodies, got %d", len(receivedBodies))
	}

	// Verify both calls received the same body
	if receivedBodies[0] != testBody || receivedBodies[1] != testBody {
		t.Errorf("expected both calls to receive %q, got %q and %q", testBody, receivedBodies[0], receivedBodies[1])
	}
}

func TestIntegration_ClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the client timeout
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(100*time.Millisecond, WithoutNewRelic())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected timeout error, but request succeeded")
	}

	// Error should be a context deadline exceeded or similar
	if !strings.Contains(err.Error(), "context deadline exceeded") &&
		!strings.Contains(err.Error(), "Client.Timeout") {
		t.Logf("got error: %v (this might be okay depending on timing)", err)
	}
}

func TestIntegration_HeadersAndRetries(t *testing.T) {
	type ctxKey string
	const userIDKey ctxKey = "user-id"

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Check headers
		apiKey := r.Header.Get("X-API-Key")
		userID := r.Header.Get("X-User-ID")

		if apiKey != "secret" || userID != "user-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if callCount < 2 {
			// First call fails temporarily
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Second call succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authenticated"))
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithHeaders(HeaderSettings{
			StaticHeaders: map[string]string{
				"X-API-Key": "secret",
			},
			ContextHeaders: map[string]any{
				"X-User-ID": userIDKey,
			},
		}),
		WithRetries(RetrySettings{
			MaxRetries:      1,
			InitialInterval: 10 * time.Millisecond,
			MaxInterval:     100 * time.Millisecond,
			Multiplier:      1.5,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.WithValue(context.Background(), userIDKey, "user-123")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls (1 failure + 1 retry), got %d", callCount)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "authenticated" {
		t.Errorf("expected body 'authenticated', got %q", string(body))
	}
	_ = resp.Body.Close()
}

func TestIntegration_ConnectionPooling(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithConnectionPool(PoolSettings{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Make multiple requests
	for i := 0; i < 3; i++ {
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		_ = resp.Body.Close()
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	client, err := NewClient(10*time.Second, WithoutNewRelic())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://invalid.example.local:99999", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	// Try to connect to non-existent server
	resp, err := client.Do(req)

	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Errorf("expected connection error, but request succeeded")
	}

	// Error should be a network error
	if !strings.Contains(err.Error(), "connection refused") &&
		!strings.Contains(err.Error(), "no such host") &&
		!strings.Contains(err.Error(), "dial") {
		t.Errorf("unexpected error, got: %v - want network error", err)
	}
}

func TestIntegration_RetryWithRetryAfterHeader(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount < 2 {
			// First call returns 429 with Retry-After
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		// Second call succeeds
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(
		10*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:           1,
			InitialInterval:      100 * time.Millisecond,
			MaxInterval:          1 * time.Second,
			Multiplier:           1.5,
			RetriableStatusCodes: []int{429},
		}),
		WithoutNewRelic(),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	_ = resp.Body.Close()

	elapsed := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}

	// Should have waited at least 1 second due to Retry-After
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected to wait ~1 second for Retry-After, but only waited %v", elapsed)
	}
}
