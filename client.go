package go_http_client

import (
	"httpclient/circuitbreaker"
	"net/http"
	"time"

	"github.com/JSainsburyPLC/go-logrus-wrapper/v2/roundtripper"
	"github.com/newrelic/go-agent/v3/newrelic"
)

var (
	DefaultTimeout = 30 * time.Second

	Default = ClientBuilder{
		Timeout:              DefaultTimeout,
		NewRelicEnabled:      true,
		SendSmartShopHeaders: true,
		CircuitBreaker: CircuitBreakerSettings{
			Enabled:  true,
			Settings: circuitbreaker.Settings{},
		},
	}
)

type CircuitBreakerSettings struct {
	Enabled  bool
	Settings circuitbreaker.Settings
}

type ClientBuilder struct {
	Timeout              time.Duration
	NewRelicEnabled      bool
	SendSmartShopHeaders bool
	CircuitBreaker       CircuitBreakerSettings
}

func (cb ClientBuilder) WithTimeout(timeout time.Duration) ClientBuilder {
	cb.Timeout = timeout
	return cb
}

func (cb ClientBuilder) DisableNewRelic() ClientBuilder {
	cb.NewRelicEnabled = false
	return cb
}

func (cb ClientBuilder) DisableSmartShopHeaders() ClientBuilder {
	cb.SendSmartShopHeaders = false
	return cb
}

func (cb ClientBuilder) DisableCircuitBreaker() ClientBuilder {
	cb.CircuitBreaker.Enabled = false
	return cb
}

func (cb ClientBuilder) WithCircuitBreakerSettings(settings circuitbreaker.Settings) ClientBuilder {
	cb.CircuitBreaker.Enabled = true
	cb.CircuitBreaker.Settings = settings
	return cb
}

func (cb ClientBuilder) Build() *http.Client {
	client := &http.Client{
		Timeout: cb.Timeout,
	}

	if cb.NewRelicEnabled {
		client.Transport = newrelic.NewRoundTripper(client.Transport)
	}

	if cb.SendSmartShopHeaders {
		client.Transport = roundtripper.Wrap(client.Transport)
	}

	if cb.CircuitBreaker.Enabled {
		client.Transport = circuitbreaker.NewRoundTripper(client.Transport, cb.CircuitBreaker.Settings)
	}

	return client
}
