package go_http_client

import (
	"errors"
	"fmt"
	"net/http"

	log "github.com/JSainsburyPLC/go-logrus-wrapper/v2"
	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
)

// CircuitBreakerKey is a strongly-typed key for circuit breakers.
// Define circuit breaker keys as constants to prevent typos.
type CircuitBreakerKey string

// String returns the string representation of the CircuitBreakerKey.
func (cbk CircuitBreakerKey) String() string {
	return string(cbk)
}

// ErrBadResponse is returned internally to trip circuit breakers on server errors.
//
// Application code should check for this with errors.Is to distinguish circuit breaker
// trips from other errors.
var ErrBadResponse = errors.New("server error")

type CircuitBreakerSettings struct {
	gobreaker.Settings
	Key        CircuitBreakerKey
	ShouldTrip func(statusCode int) bool
}

type circuitBreakerConfig struct {
	breaker    *gobreaker.CircuitBreaker[*http.Response]
	shouldTrip func(statusCode int) bool
}

func newCircuitBreakers(cbSettings map[CircuitBreakerKey]CircuitBreakerSettings) map[CircuitBreakerKey]*circuitBreakerConfig {
	breakers := make(map[CircuitBreakerKey]*circuitBreakerConfig)
	for key, settings := range cbSettings {
		shouldTrip := settings.ShouldTrip
		if shouldTrip == nil {
			shouldTrip = func(statusCode int) bool {
				return statusCode >= http.StatusInternalServerError
			}
		}

		if settings.OnStateChange == nil {
			settings.OnStateChange = logCBStateChange
		}

		if settings.Name == "" {
			settings.Name = key.String()
		}

		breakers[key] = &circuitBreakerConfig{
			breaker:    gobreaker.NewCircuitBreaker[*http.Response](settings.Settings), //nolint:bodyclose
			shouldTrip: shouldTrip,
		}
	}

	return breakers
}

// GetBreaker returns the circuit breaker for the given key.
// Panics if the circuit breaker is not configured.
func (c *HTTPClient) GetBreaker(key CircuitBreakerKey) *gobreaker.CircuitBreaker[*http.Response] {
	cfg, exists := c.breakers[key]
	if !exists {
		panic(fmt.Sprintf("circuit breaker %q not configured", key))
	}

	return cfg.breaker
}

// ShouldTrip returns true if the given status code should trip the circuit breaker.
// Uses the custom ShouldTrip function if set, otherwise defaults to >= 500.
// Panics if the circuit breaker is not configured (name typo, etc).
func (c *HTTPClient) ShouldTrip(key CircuitBreakerKey, statusCode int) bool {
	cfg, exists := c.breakers[key]
	if !exists {
		panic(fmt.Sprintf("circuit breaker %q not configured", key))
	}

	return cfg.shouldTrip(statusCode)
}

// ExecuteWithBreaker is a helper function that executes a function with circuit breaker protection.
// It automatically handles the ShouldTrip logic and returns ErrBadResponse when appropriate.
//
// This reduces boilerplate when using circuit breakers with oapi-codegen or other HTTP clients.
//
// Example usage:
//
//	resp, err := client.ExecuteWithBreaker(BreakerUsersGet, func() (*http.Response, error) {
//	    oapiResp, err := apiClient.GetUserWithResponse(ctx, userID)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return oapiResp.HTTPResponse, nil
//	})
//	if err != nil {
//	    if errors.Is(err, gobreaker.ErrOpenState) {
//	        // Circuit breaker is open, handle gracefully
//	    }
//	    return nil, err
//	}
//
// Panics if the circuit breaker is not configured.
func (c *HTTPClient) ExecuteWithBreaker(
	key CircuitBreakerKey,
	fn func() (*http.Response, error),
) (*http.Response, error) {
	breaker := c.GetBreaker(key)

	return breaker.Execute(func() (*http.Response, error) {
		resp, err := fn()
		if err != nil {
			return nil, err
		}

		if c.ShouldTrip(key, resp.StatusCode) {
			return resp, ErrBadResponse
		}

		return resp, nil
	})
}

func logCBStateChange(name string, from gobreaker.State, to gobreaker.State) {
	log.WithFields(logrus.Fields{
		"circuit_breaker": name,
		"from_state":      from.String(),
		"to_state":        to.String(),
	}).Error("circuit breaker changed state")
}
