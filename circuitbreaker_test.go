package go_http_client

import (
	"errors"
	"net/http"
	"testing"

	"github.com/sony/gobreaker/v2"
)

func TestCircuitBreakerKey_String(t *testing.T) {
	tests := []struct {
		name string
		key  CircuitBreakerKey
		want string
	}{
		{
			name: "simple key",
			key:  CircuitBreakerKey("user-service"),
			want: "user-service",
		},
		{
			name: "key with colons",
			key:  CircuitBreakerKey("service:endpoint:version"),
			want: "service:endpoint:version",
		},
		{
			name: "empty key",
			key:  CircuitBreakerKey(""),
			want: "",
		},
		{
			name: "key with special characters",
			key:  CircuitBreakerKey("service-v2.0_prod"),
			want: "service-v2.0_prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.key.String()
			if result != tt.want {
				t.Errorf("String(): expected %q, got %q", tt.want, result)
			}
		})
	}
}

func TestNewCircuitBreakers_Empty(t *testing.T) {
	settings := make(map[CircuitBreakerKey]CircuitBreakerSettings)
	breakers := newCircuitBreakers(settings)

	if len(breakers) != 0 {
		t.Errorf("expected empty breakers map, got %d", len(breakers))
	}
}

func TestNewCircuitBreakers_SingleBreaker(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key: testKey,
			Settings: gobreaker.Settings{
				Name: "test",
			},
		},
	}

	breakers := newCircuitBreakers(settings)

	if len(breakers) != 1 {
		t.Errorf("expected 1 breaker, got %d", len(breakers))
	}

	if _, exists := breakers[testKey]; !exists {
		t.Errorf("expected breaker for key %q", testKey)
	}
}

func TestNewCircuitBreakers_DefaultShouldTrip(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key: testKey,
			Settings: gobreaker.Settings{
				Name: "test",
			},
			// ShouldTrip is nil, should use default
		},
	}

	breakers := newCircuitBreakers(settings)
	cfg := breakers[testKey]

	tests := []struct {
		statusCode int
		shouldTrip bool
	}{
		{200, false}, // OK
		{404, false}, // Not Found
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
		{499, false}, // Custom 4xx
		{600, true},  // > 500
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := cfg.shouldTrip(tt.statusCode)
			if result != tt.shouldTrip {
				t.Errorf("statusCode %d: expected shouldTrip %v, got %v", tt.statusCode, tt.shouldTrip, result)
			}
		})
	}
}

func TestNewCircuitBreakers_CustomShouldTrip(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	customShouldTrip := func(statusCode int) bool {
		return statusCode == 429 || statusCode >= 500
	}

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key: testKey,
			Settings: gobreaker.Settings{
				Name: "test",
			},
			ShouldTrip: customShouldTrip,
		},
	}

	breakers := newCircuitBreakers(settings)
	cfg := breakers[testKey]

	tests := []struct {
		statusCode int
		shouldTrip bool
	}{
		{200, false}, // OK
		{429, true},  // Too Many Requests (custom)
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable (>= 500)
		{404, false}, // Not Found
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := cfg.shouldTrip(tt.statusCode)
			if result != tt.shouldTrip {
				t.Errorf("statusCode %d: expected shouldTrip %v, got %v", tt.statusCode, tt.shouldTrip, result)
			}
		})
	}
}

func TestNewCircuitBreakers_DefaultName(t *testing.T) {
	const testKey CircuitBreakerKey = "my-service"

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key:      testKey,
			Settings: gobreaker.Settings{},
		},
	}

	breakers := newCircuitBreakers(settings)
	cfg := breakers[testKey]

	if cfg.breaker.Name() != "my-service" {
		t.Errorf("expected name 'my-service', got %q", cfg.breaker.Name())
	}
}

func TestNewCircuitBreakers_CustomName(t *testing.T) {
	const testKey CircuitBreakerKey = "my-service"

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key: testKey,
			Settings: gobreaker.Settings{
				Name: "custom-name",
			},
		},
	}

	breakers := newCircuitBreakers(settings)
	cfg := breakers[testKey]

	if cfg.breaker.Name() != "custom-name" {
		t.Errorf("expected name 'custom-name', got %q", cfg.breaker.Name())
	}
}

func TestNewCircuitBreakers_MultipleBreakers(t *testing.T) {
	keys := []CircuitBreakerKey{
		"service-1",
		"service-2",
		"service-3",
	}

	settings := make(map[CircuitBreakerKey]CircuitBreakerSettings)
	for _, key := range keys {
		settings[key] = CircuitBreakerSettings{
			Key: key,
			Settings: gobreaker.Settings{
				Name: key.String(),
			},
		}
	}

	breakers := newCircuitBreakers(settings)

	if len(breakers) != len(keys) {
		t.Errorf("expected %d breakers, got %d", len(keys), len(breakers))
	}

	for _, key := range keys {
		if _, exists := breakers[key]; !exists {
			t.Errorf("expected breaker for key %q", key)
		}
	}
}

func TestGetBreaker_Success(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker:    gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(int) bool { return true },
			},
		},
	}

	breaker := client.GetBreaker(testKey)

	if breaker == nil {
		t.Errorf("expected breaker, got nil")
	}
}

func TestGetBreaker_NotConfigured(t *testing.T) {
	client := &HTTPClient{
		breakers: make(map[CircuitBreakerKey]*circuitBreakerConfig),
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when breaker not configured")
		}
	}()

	client.GetBreaker("non-existent")
	t.Errorf("expected panic")
}

func TestShouldTrip_WithDefaultBehavior(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode >= http.StatusInternalServerError
				},
			},
		},
	}

	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{404, false},
		{500, true},
		{502, true},
		{503, true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := client.ShouldTrip(testKey, tt.statusCode)
			if result != tt.expected {
				t.Errorf("statusCode %d: expected %v, got %v", tt.statusCode, tt.expected, result)
			}
		})
	}
}

func TestShouldTrip_WithCustomBehavior(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	customShouldTrip := func(statusCode int) bool {
		return statusCode == 429 || statusCode >= 502
	}

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker:    gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: customShouldTrip,
			},
		},
	}

	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{429, true},
		{500, false},
		{502, true},
		{503, true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			result := client.ShouldTrip(testKey, tt.statusCode)
			if result != tt.expected {
				t.Errorf("statusCode %d: expected %v, got %v", tt.statusCode, tt.expected, result)
			}
		})
	}
}

func TestShouldTrip_NotConfigured(t *testing.T) {
	client := &HTTPClient{
		breakers: make(map[CircuitBreakerKey]*circuitBreakerConfig),
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when breaker not configured")
		}
	}()

	client.ShouldTrip("non-existent", 500)
	t.Errorf("expected panic")
}

func TestCircuitBreakerKey_TypeSafety(t *testing.T) {
	key1 := CircuitBreakerKey("service-a")
	key2 := CircuitBreakerKey("service-a")

	m := make(map[CircuitBreakerKey]string)
	m[key1] = "value"

	if m[key2] != "value" {
		t.Errorf("key lookup should work")
	}
}

func TestNewCircuitBreakers_PresetsDefaults(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	settings := map[CircuitBreakerKey]CircuitBreakerSettings{
		testKey: {
			Key:      testKey,
			Settings: gobreaker.Settings{
				// Name and OnStateChange are empty
			},
		},
	}

	breakers := newCircuitBreakers(settings)
	cfg := breakers[testKey]

	if cfg.breaker.Name() != testKey.String() {
		t.Errorf("expected name to default to key string")
	}

	if cfg.shouldTrip == nil {
		t.Errorf("expected shouldTrip to be set to default function")
	}

	// Verify default shouldTrip works
	if !cfg.shouldTrip(500) {
		t.Errorf("expected default shouldTrip to return true for 500")
	}

	if cfg.shouldTrip(404) {
		t.Errorf("expected default shouldTrip to return false for 404")
	}
}

func TestExecuteWithBreaker_Success(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode >= 500
				},
			},
		},
	}

	resp, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestExecuteWithBreaker_NetworkError(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode >= 500
				},
			},
		},
	}

	expectedErr := http.ErrServerClosed

	_, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return nil, expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestExecuteWithBreaker_TripsOnBadStatus(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode >= 500
				},
			},
		},
	}

	resp, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
		}, nil
	})

	if !errors.Is(err, ErrBadResponse) {
		t.Errorf("expected ErrBadResponse, got %v", err)
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
}

func TestExecuteWithBreaker_DoesNotTripOnGoodStatus(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode >= 500
				},
			},
		},
	}

	resp, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
		}, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestExecuteWithBreaker_CustomShouldTrip(t *testing.T) {
	const testKey CircuitBreakerKey = "test-service"

	client := &HTTPClient{
		breakers: map[CircuitBreakerKey]*circuitBreakerConfig{
			testKey: {
				breaker: gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{}),
				shouldTrip: func(statusCode int) bool {
					return statusCode == 429 || statusCode >= 500
				},
			},
		},
	}

	// Test 429 trips
	_, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests}, nil
	})

	if !errors.Is(err, ErrBadResponse) {
		t.Errorf("expected ErrBadResponse for 429, got %v", err)
	}

	// Test 404 doesn't trip
	resp, err := client.ExecuteWithBreaker(testKey, func() (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound}, nil
	})

	if err != nil {
		t.Errorf("expected no error for 404, got %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestExecuteWithBreaker_NotConfigured(t *testing.T) {
	client := &HTTPClient{
		breakers: make(map[CircuitBreakerKey]*circuitBreakerConfig),
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when breaker not configured")
		}
	}()

	_, _ = client.ExecuteWithBreaker("non-existent", func() (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})

	t.Errorf("expected panic")
}
