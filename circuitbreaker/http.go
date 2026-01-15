package circuitbreaker

import (
	"errors"
	"net/http"

	log "github.com/JSainsburyPLC/go-logrus-wrapper/v2"
	"github.com/sirupsen/logrus"

	"github.com/sony/gobreaker/v2"
)

type Settings struct {
	gobreaker.Settings
	ShouldTrip func(statusCode int) bool
}

type circuitBreakerTransport struct {
	wrapped    http.RoundTripper
	cb         *gobreaker.CircuitBreaker[*http.Response]
	shouldTrip func(statusCode int) bool
}

func NewRoundTripper(wrapped http.RoundTripper, settings Settings) http.RoundTripper {
	if settings.OnStateChange == nil {
		settings.OnStateChange = logCBStateChange
	}

	if settings.ShouldTrip == nil {
		settings.ShouldTrip = func(statusCode int) bool {
			return statusCode >= http.StatusInternalServerError
		}
	}

	return &circuitBreakerTransport{
		wrapped:    wrapped,
		cb:         gobreaker.NewCircuitBreaker[*http.Response](settings.Settings),
		shouldTrip: settings.ShouldTrip,
	}
}

var errBadResponse = errors.New("server error")

func (t circuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.cb.Execute(func() (*http.Response, error) {
		resp, err := t.wrapped.RoundTrip(req)
		if resp != nil && t.shouldTrip(resp.StatusCode) {
			return resp, errBadResponse
		}

		return resp, err
	})

	// If the server returns an error, suppress and let the HTTP client caller
	// decide how to handle the response body. This error is used internally
	// to force the circuit breaker to trip when the server returns a 5XX response.
	if errors.Is(err, errBadResponse) {
		return resp, nil
	}

	return resp, err
}

func logCBStateChange(name string, from gobreaker.State, to gobreaker.State) {
	log.WithFields(logrus.Fields{
		"circuit_breaker": name,
		"from_state":      from.String(),
		"to_state":        to.String(),
	}).Error("circuit breaker changed state")
}
