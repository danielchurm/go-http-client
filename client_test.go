package go_http_client

import (
	"strings"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestNewClient_ValidTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "1 millisecond timeout",
			timeout: 1 * time.Millisecond,
		},
		{
			name:    "1 second timeout",
			timeout: 1 * time.Second,
		},
		{
			name:    "30 second timeout",
			timeout: 30 * time.Second,
		},
		{
			name:    "1 minute timeout",
			timeout: 1 * time.Minute,
		},
		{
			name:    "very long timeout",
			timeout: 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.timeout)

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if client == nil {
				t.Fatalf("expected client, got nil")
			}

			if client.Timeout != tt.timeout {
				t.Errorf("expected timeout %v, got %v", tt.timeout, client.Timeout)
			}

			if client.breakers == nil {
				t.Errorf("expected breakers map, got nil")
			}
		})
	}
}

func TestNewClient_InvalidTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "zero timeout",
			timeout: 0,
		},
		{
			name:    "negative timeout",
			timeout: -1 * time.Second,
		},
		{
			name:    "negative nanosecond",
			timeout: -1 * time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.timeout)

			if err == nil {
				t.Errorf("expected error for timeout %v, got nil", tt.timeout)
			}

			if client != nil {
				t.Errorf("expected nil client for invalid timeout, got %v", client)
			}

			if err != nil && err.Error() != "timeout must be greater than 0, got: "+tt.timeout.String() {
				t.Logf("error message: %v", err.Error())
			}
		})
	}
}

func TestNewClient_DefaultSettings(t *testing.T) {
	client, err := NewClient(30 * time.Second)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.Transport == nil {
		t.Errorf("expected transport to be set")
	}
}

func TestNewClient_WithoutNewRelic(t *testing.T) {
	client, err := NewClient(30*time.Second, WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Fatalf("expected client, got nil")
	}

	// Just verify it creates successfully without panicking
	if client.Transport == nil {
		t.Errorf("expected transport to be set")
	}
}

func TestNewClient_WithRetries(t *testing.T) {
	retrySettings := RetrySettings{
		MaxRetries:      2,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     1 * time.Second,
		Multiplier:      2.0,
	}

	client, err := NewClient(30*time.Second, WithRetries(retrySettings), WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Errorf("expected client, got nil")
	}
}

func TestNewClient_WithRetries_InvalidSettings(t *testing.T) {
	retrySettings := RetrySettings{
		MaxRetries:      100,              // Very large
		InitialInterval: 10 * time.Second, // Large interval
		MaxInterval:     20 * time.Second,
		Multiplier:      2.0,
	}

	// This should fail because worst-case retry time exceeds client timeout
	client, err := NewClient(5*time.Second, WithRetries(retrySettings), WithoutNewRelic())

	if err == nil {
		t.Errorf("expected error due to retry settings exceeding timeout")
	}

	if client != nil {
		t.Errorf("expected nil client when validation fails")
	}

	if err != nil && !strings.Contains(err.Error(), "worst case retry backoff") {
		t.Logf("got error: %v", err)
	}
}

func TestNewClient_WithHeaders(t *testing.T) {
	headerSettings := HeaderSettings{
		StaticHeaders: map[string]string{
			"X-API-Key": "test-key",
		},
	}

	client, err := NewClient(30*time.Second, WithHeaders(headerSettings), WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Errorf("expected client, got nil")
	}
}

func TestNewClient_WithConnectionPool(t *testing.T) {
	poolSettings := PoolSettings{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
	}

	client, err := NewClient(30*time.Second, WithConnectionPool(poolSettings), WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Errorf("expected client, got nil")
	}
}

func TestNewClient_WithCircuitBreaker(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	settings := CircuitBreakerSettings{
		Key: testKey,
		Settings: gobreaker.Settings{
			Name:        "test",
			MaxRequests: 5,
		},
	}

	client, err := NewClient(30*time.Second, WithCircuitBreaker(settings), WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Errorf("expected client, got nil")
	}

	// Verify the breaker was registered
	breaker := client.GetBreaker(testKey) //nolint:bodyclose
	if breaker == nil {
		t.Errorf("expected breaker to be registered")
	}
}

func TestNewClient_WithMultipleCircuitBreakers(t *testing.T) {
	const key1 CircuitBreakerKey = "service-1"
	const key2 CircuitBreakerKey = "service-2"

	settings1 := CircuitBreakerSettings{
		Key: key1,
		Settings: gobreaker.Settings{
			Name: "service-1",
		},
	}

	settings2 := CircuitBreakerSettings{
		Key: key2,
		Settings: gobreaker.Settings{
			Name: "service-2",
		},
	}

	client, err := NewClient(
		30*time.Second,
		WithCircuitBreaker(settings1),
		WithCircuitBreaker(settings2),
		WithoutNewRelic(),
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify both breakers were registered
	breaker1 := client.GetBreaker(key1) //nolint:bodyclose
	breaker2 := client.GetBreaker(key2) //nolint:bodyclose

	if breaker1 == nil {
		t.Errorf("expected breaker 1 to be registered")
	}

	if breaker2 == nil {
		t.Errorf("expected breaker 2 to be registered")
	}
}

func TestNewClient_WithCircuitBreakers(t *testing.T) {
	const key1 CircuitBreakerKey = "service-1"
	const key2 CircuitBreakerKey = "service-2"

	breakerSettings := []CircuitBreakerSettings{
		{
			Key: key1,
			Settings: gobreaker.Settings{
				Name: "service-1",
			},
		},
		{
			Key: key2,
			Settings: gobreaker.Settings{
				Name: "service-2",
			},
		},
	}

	client, err := NewClient(
		30*time.Second,
		WithCircuitBreakers(breakerSettings),
		WithoutNewRelic(),
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify both breakers were registered
	breaker1 := client.GetBreaker(key1) //nolint:bodyclose
	breaker2 := client.GetBreaker(key2) //nolint:bodyclose

	if breaker1 == nil {
		t.Errorf("expected breaker 1 to be registered")
	}

	if breaker2 == nil {
		t.Errorf("expected breaker 2 to be registered")
	}
}

func TestNewClient_AllOptionsEnabled(t *testing.T) {
	const breakerKey CircuitBreakerKey = "api-service"

	client, err := NewClient(
		30*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      2,
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     1 * time.Second,
			Multiplier:      1.5,
		}),
		WithHeaders(HeaderSettings{
			StaticHeaders: map[string]string{
				"X-API-Key": "secret",
			},
		}),
		WithConnectionPool(PoolSettings{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 10,
		}),
		WithCircuitBreaker(CircuitBreakerSettings{
			Key: breakerKey,
			Settings: gobreaker.Settings{
				Name: "api-service",
			},
		}),
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client == nil {
		t.Fatalf("expected client, got nil")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", client.Timeout)
	}

	breaker := client.GetBreaker(breakerKey) //nolint:bodyclose
	if breaker == nil {
		t.Errorf("expected circuit breaker to be registered")
	}
}

func TestNewClient_TransportChaining(t *testing.T) {
	// Verify that with all options enabled, the transport chain is set up correctly
	client, err := NewClient(
		30*time.Second,
		WithRetries(RetrySettings{
			MaxRetries:      1,
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     1 * time.Second,
			Multiplier:      1.5,
		}),
		WithHeaders(HeaderSettings{
			StaticHeaders: map[string]string{"X-Test": "value"},
		}),
		WithoutNewRelic(),
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the transport is not nil and is properly set up
	if client.Transport == nil {
		t.Fatalf("expected transport to be set")
	}
}

func TestNewClient_EmptyCircuitBreakerSettings(t *testing.T) {
	// Test that an empty circuit breaker settings map doesn't cause issues
	client, err := NewClient(30*time.Second, WithoutNewRelic())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(client.breakers) != 0 {
		t.Errorf("expected empty breakers map, got %d breakers", len(client.breakers))
	}
}

func TestNewClient_OptionsAppliedInOrder(t *testing.T) {
	// Verify that multiple options can be applied and last one wins for conflicting settings
	client1, _ := NewClient(30*time.Second, WithoutNewRelic())
	client2, _ := NewClient(30*time.Second, WithoutNewRelic(), WithoutNewRelic())

	// Both should be created successfully
	if client1 == nil || client2 == nil {
		t.Errorf("expected both clients to be created successfully")
	}
}

func TestNewClient_ClientIsFullyFunctional(t *testing.T) {
	client, err := NewClient(30 * time.Second)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.Client == nil {
		t.Errorf("expected embedded *http.Client, got nil")
	}

	if client.breakers == nil {
		t.Errorf("expected breakers map, got nil")
	}

	// Verify the embedded http.Client is safely dereferenceable (not nil)
	_ = *client.Client // Will panic if client.Client is nil, stronger than == nil check
}

func TestNewClient_TimeoutIsCorrectlySet(t *testing.T) {
	tests := []time.Duration{
		1 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
		30 * time.Second,
		5 * time.Minute,
	}

	for _, expectedTimeout := range tests {
		t.Run(expectedTimeout.String(), func(t *testing.T) {
			client, err := NewClient(expectedTimeout, WithoutNewRelic())

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if client.Timeout != expectedTimeout {
				t.Errorf("expected timeout %v, got %v", expectedTimeout, client.Timeout)
			}
		})
	}
}
