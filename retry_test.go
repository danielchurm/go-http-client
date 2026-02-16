package go_http_client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

type trackingRoundTripper struct {
	requestCount int
	requests     []*http.Request
	responses    []*http.Response
	errors       []error
	nextIdx      int
}

func (t *trackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requests = append(t.requests, req)
	t.requestCount++

	// Read and restore body so we can inspect it
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	idx := t.nextIdx
	t.nextIdx++

	if idx < len(t.errors) && t.errors[idx] != nil {
		return nil, t.errors[idx]
	}

	if idx < len(t.responses) && t.responses[idx] != nil {
		return t.responses[idx], nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestRetryTransport_IdempotentMethods(t *testing.T) {
	tests := []struct {
		method       string
		isIdempotent bool
	}{
		{http.MethodGet, true},
		{http.MethodHead, true},
		{http.MethodOptions, true},
		{http.MethodTrace, true},
		{http.MethodPut, true},
		{http.MethodDelete, true},
		{http.MethodPost, false},
		{http.MethodPatch, false},
		{"CUSTOM", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_idempotent=%v", tt.method, tt.isIdempotent), func(t *testing.T) {
			if result := isIdempotent(tt.method); result != tt.isIdempotent {
				t.Errorf("isIdempotent(%q): expected %v, got %v", tt.method, tt.isIdempotent, result)
			}
		})
	}
}

func TestRetryTransport_NonIdempotentMethodsNotRetried(t *testing.T) {
	tests := []struct {
		method string
	}{
		{http.MethodPost},
		{http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			mock := &trackingRoundTripper{
				responses: []*http.Response{
					{
						StatusCode: http.StatusServiceUnavailable, // 503 - normally retryable
						Body:       io.NopCloser(strings.NewReader("")),
					},
				},
			}

			transport := newRetryTransport(mock, RetrySettings{
				MaxRetries:           3,
				RetriableStatusCodes: []int{503},
			})

			req, _ := http.NewRequestWithContext(t.Context(), tt.method, "http://example.com", nil)
			resp, err := transport.RoundTrip(req)

			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}

			_ = resp.Body.Close()

			if mock.requestCount != 1 {
				t.Errorf("expected 1 request, got %d. %s requests should not be retried", mock.requestCount, tt.method)
			}

			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("expected status 503, got %d", resp.StatusCode)
			}
		})
	}
}

func TestRetryTransport_SuccessfulResponseNotRetried(t *testing.T) {
	mock := &trackingRoundTripper{
		responses: []*http.Response{
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			},
		},
	}

	transport := newRetryTransport(mock, RetrySettings{
		MaxRetries:           3,
		RetriableStatusCodes: []int{503},
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	_ = resp.Body.Close()

	if mock.requestCount != 1 {
		t.Errorf("expected 1 request, got %d. successful responses should not be retried", mock.requestCount)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryTransport_RetriableStatusCodes(t *testing.T) {
	tests := []struct {
		name                 string
		statusCode           int
		retriableStatusCodes []int
		shouldRetry          bool
		responses            int // number of responses to provide
	}{
		{
			name:                 "default retriable status codes",
			statusCode:           http.StatusServiceUnavailable,
			retriableStatusCodes: nil, // uses defaults
			shouldRetry:          true,
			responses:            2, // first fails, second succeeds
		},
		{
			name:                 "429 Too Many Requests is retriable by default",
			statusCode:           http.StatusTooManyRequests,
			retriableStatusCodes: nil,
			shouldRetry:          true,
			responses:            2,
		},
		{
			name:                 "502 Bad Gateway is retriable by default",
			statusCode:           http.StatusBadGateway,
			retriableStatusCodes: nil,
			shouldRetry:          true,
			responses:            2,
		},
		{
			name:                 "504 Gateway Timeout is retriable by default",
			statusCode:           http.StatusGatewayTimeout,
			retriableStatusCodes: nil,
			shouldRetry:          true,
			responses:            2,
		},
		{
			name:                 "500 Internal Server Error is not retriable by default",
			statusCode:           http.StatusInternalServerError,
			retriableStatusCodes: nil,
			shouldRetry:          false,
			responses:            1,
		},
		{
			name:                 "custom retriable status code",
			statusCode:           http.StatusInternalServerError,
			retriableStatusCodes: []int{500, 502},
			shouldRetry:          true,
			responses:            2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := make([]*http.Response, tt.responses)
			for i := 0; i < tt.responses-1; i++ {
				responses[i] = &http.Response{
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(strings.NewReader("")),
				}
			}
			// Last response is always success
			responses[tt.responses-1] = &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			}

			mock := &trackingRoundTripper{responses: responses}

			settings := RetrySettings{
				MaxRetries: 5,
			}
			if tt.retriableStatusCodes != nil {
				settings.RetriableStatusCodes = tt.retriableStatusCodes
			}

			transport := newRetryTransport(mock, settings)

			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
			resp, err := transport.RoundTrip(req)

			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}

			_ = resp.Body.Close()

			if tt.shouldRetry && mock.requestCount == 1 {
				t.Errorf("expected retry for status %d, but got %d requests", tt.statusCode, mock.requestCount)
			}

			if !tt.shouldRetry && mock.requestCount != 1 {
				t.Errorf("expected no retry for status %d, but got %d requests", tt.statusCode, mock.requestCount)
			}
		})
	}
}

func TestRetryTransport_MaxRetriesRespected(t *testing.T) {
	tests := []struct {
		name         string
		maxRetries   int
		expectedReqs int
	}{
		{
			name:         "one retry means two attempts",
			maxRetries:   1,
			expectedReqs: 2,
		},
		{
			name:         "two retries means three attempts",
			maxRetries:   2,
			expectedReqs: 3,
		},
		{
			name:         "three retries means four attempts",
			maxRetries:   3,
			expectedReqs: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create enough failing responses
			responses := make([]*http.Response, tt.expectedReqs+10)
			for i := range responses {
				responses[i] = &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader("")),
				}
			}

			mock := &trackingRoundTripper{responses: responses}

			transport := newRetryTransport(mock, RetrySettings{
				MaxRetries:           tt.maxRetries,
				InitialInterval:      1 * time.Millisecond,
				MaxInterval:          1 * time.Millisecond,
				Multiplier:           1.5,
				RetriableStatusCodes: []int{503},
			})

			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req = req.WithContext(ctx)

			_, err := transport.RoundTrip(req) //nolint:bodyclose

			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			if mock.requestCount != tt.expectedReqs {
				t.Errorf("expected %d requests, got %d", tt.expectedReqs, mock.requestCount)
			}
		})
	}
}

func TestRetryTransport_NetworkErrorsRetried(t *testing.T) {
	mock := &trackingRoundTripper{
		errors: []error{
			errors.New("connection refused"),
			errors.New("connection refused"),
			nil, // third attempt succeeds
		},
		responses: []*http.Response{
			nil, // ignored since error happens first
			nil, // ignored
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			},
		},
	}

	transport := newRetryTransport(mock, RetrySettings{
		MaxRetries:      3,
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     1 * time.Millisecond,
		Multiplier:      1.5,
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	resp, err := transport.RoundTrip(req)

	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	_ = resp.Body.Close()

	if mock.requestCount != 3 {
		t.Errorf("expected 3 requests (2 retries after network error), got %d", mock.requestCount)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryTransport_ContextCancellation(t *testing.T) {
	mock := &trackingRoundTripper{
		responses: []*http.Response{
			{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
			},
		},
	}

	transport := newRetryTransport(mock, RetrySettings{
		MaxRetries:           10,
		InitialInterval:      100 * time.Millisecond,
		MaxInterval:          100 * time.Millisecond,
		Multiplier:           1.5,
		RetriableStatusCodes: []int{503},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)

	resp, err := transport.RoundTrip(req) //nolint:bodyclose

	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}

	// Should have made one or more attempts but not all 11
	if mock.requestCount > 5 {
		t.Errorf("expected context cancellation to prevent many retries, got %d requests", mock.requestCount)
	}

	if resp != nil {
		t.Errorf("expected nil response on context cancellation, got %v", resp)
	}
}

func TestRetryTransport_RequestBodyPreserved(t *testing.T) {
	const bodyContent = "test request body"

	mock := &trackingRoundTripper{
		responses: []*http.Response{
			{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("")),
			},
			{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			},
		},
	}

	transport := newRetryTransport(mock, RetrySettings{
		MaxRetries:           1,
		InitialInterval:      1 * time.Millisecond,
		MaxInterval:          1 * time.Millisecond,
		Multiplier:           1.5,
		RetriableStatusCodes: []int{503},
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, "http://example.com", strings.NewReader(bodyContent))
	resp, err := transport.RoundTrip(req)

	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if mock.requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", mock.requestCount)
	}

	for i, req := range mock.requests {
		body, _ := io.ReadAll(req.Body)
		if string(body) != bodyContent {
			t.Errorf("request %d body: expected %q, got %q", i+1, bodyContent, string(body))
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name           string
		headerValue    string
		expectPositive bool
		maxExpected    time.Duration
	}{
		{
			name:           "empty header",
			headerValue:    "",
			expectPositive: false,
		},
		{
			name:           "seconds as integer",
			headerValue:    "30",
			expectPositive: true,
			maxExpected:    31 * time.Second,
		},
		{
			name:           "single second",
			headerValue:    "1",
			expectPositive: true,
			maxExpected:    2 * time.Second,
		},
		{
			name:           "max allowed seconds (300)",
			headerValue:    "300",
			expectPositive: true,
			maxExpected:    301 * time.Second,
		},
		{
			name:           "seconds exceeding max (301) returns 0",
			headerValue:    "301",
			expectPositive: false,
			maxExpected:    300 * time.Second,
		},
		{
			name:           "zero seconds returns 0",
			headerValue:    "0",
			expectPositive: false,
			maxExpected:    0,
		},
		{
			name:           "negative seconds returns 0",
			headerValue:    "-10",
			expectPositive: false,
			maxExpected:    0,
		},
		{
			name:           "non-integer non-date value returns 0",
			headerValue:    "invalid",
			expectPositive: false,
			maxExpected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfter(tt.headerValue)

			if tt.expectPositive {
				if result <= 0 {
					t.Errorf("expected positive duration, got %v", result)
				}
				if result > tt.maxExpected {
					t.Errorf("expected duration <= %v, got %v", tt.maxExpected, result)
				}
			} else {
				if result != 0 {
					t.Errorf("expected 0 duration, got %v", result)
				}
			}
		})
	}
}

func TestRetryTransport_RetryAfterHeader(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		retryAfter      string
		shouldRespectRA bool
	}{
		{
			name:            "Retry-After respected for 429",
			statusCode:      http.StatusTooManyRequests,
			retryAfter:      "1",
			shouldRespectRA: true,
		},
		{
			name:            "Retry-After respected for 503",
			statusCode:      http.StatusServiceUnavailable,
			retryAfter:      "1",
			shouldRespectRA: true,
		},
		{
			name:            "Retry-After ignored for 502",
			statusCode:      http.StatusBadGateway,
			retryAfter:      "1",
			shouldRespectRA: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstResp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}
			firstResp.Header.Set("Retry-After", tt.retryAfter)

			secondResp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			}

			mock := &trackingRoundTripper{
				responses: []*http.Response{firstResp, secondResp},
			}

			start := time.Now()
			transport := newRetryTransport(mock, RetrySettings{
				MaxRetries:           1,
				InitialInterval:      1 * time.Millisecond,
				MaxInterval:          1 * time.Millisecond,
				Multiplier:           1.5,
				RetriableStatusCodes: []int{429, 502, 503},
			})

			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
			resp, err := transport.RoundTrip(req)

			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}

			_ = resp.Body.Close()

			elapsed := time.Since(start)

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", resp.StatusCode)
			}

			if tt.shouldRespectRA && elapsed < 900*time.Millisecond {
				t.Errorf("expected delay for Retry-After, got only %v", elapsed)
			}
		})
	}
}

func TestValidateRetrySettings(t *testing.T) {
	tests := []struct {
		name          string
		settings      RetrySettings
		clientTimeout time.Duration
		shouldError   bool
		errorContains string
	}{
		{
			name: "valid default settings",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: 500 * time.Millisecond,
				MaxInterval:     60 * time.Second,
				Multiplier:      1.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   false,
		},
		{
			name: "negative MaxRetries fails validation",
			settings: RetrySettings{
				MaxRetries:      -1,
				InitialInterval: 500 * time.Millisecond,
				MaxInterval:     60 * time.Second,
				Multiplier:      1.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   true,
			errorContains: "max retries must be >= 0",
		},
		{
			name: "zero InitialInterval becomes default (valid)",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: 0,
				MaxInterval:     60 * time.Second,
				Multiplier:      1.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   false,
		},
		{
			name: "negative InitialInterval fails validation",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: -1 * time.Second,
				MaxInterval:     60 * time.Second,
				Multiplier:      1.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   true,
			errorContains: "initial interval must be > 0",
		},
		{
			name: "MaxInterval less than InitialInterval fails validation",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: 1 * time.Second,
				MaxInterval:     500 * time.Millisecond,
				Multiplier:      1.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   true,
			errorContains: "must be >=",
		},
		{
			name: "Multiplier < 1.0 fails validation",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: 500 * time.Millisecond,
				MaxInterval:     60 * time.Second,
				Multiplier:      0.9,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   true,
			errorContains: "multiplier must be >= 1.0",
		},
		{
			name: "Multiplier < 1.0 (0.5) fails validation",
			settings: RetrySettings{
				MaxRetries:      3,
				InitialInterval: 500 * time.Millisecond,
				MaxInterval:     60 * time.Second,
				Multiplier:      0.5,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   true,
			errorContains: "multiplier must be >= 1.0",
		},
		{
			name: "worst case retry time exceeds client timeout",
			settings: RetrySettings{
				MaxRetries:      5,
				InitialInterval: 5 * time.Second,
				MaxInterval:     10 * time.Second,
				Multiplier:      2.0,
			},
			clientTimeout: 10 * time.Second,
			shouldError:   true,
			errorContains: "worst case retry backoff",
		},
		{
			name: "high MaxRetries with large intervals exceeds timeout",
			settings: RetrySettings{
				MaxRetries:      10,
				InitialInterval: 1 * time.Second,
				MaxInterval:     5 * time.Second,
				Multiplier:      1.5,
			},
			clientTimeout: 5 * time.Second,
			shouldError:   true,
			errorContains: "worst case retry backoff",
		},
		{
			name: "uses defaults when zero values provided",
			settings: RetrySettings{
				MaxRetries:      0,
				InitialInterval: 0,
				MaxInterval:     0,
				Multiplier:      0,
			},
			clientTimeout: 30 * time.Second,
			shouldError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRetrySettings(tt.settings, tt.clientTimeout)

			if tt.shouldError && err == nil {
				t.Errorf("expected error, got nil")
			}

			if !tt.shouldError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			if tt.shouldError && tt.errorContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			}
		})
	}
}

func TestParseRetryAfter_HTTPDateFormat(t *testing.T) {
	futureTime := time.Now().Add(2 * time.Second)
	futureHeader := futureTime.Format(http.TimeFormat)

	result := parseRetryAfter(futureHeader)

	if result <= 0 {
		t.Errorf("expected positive duration for future date, got %v", result)
	}

	// Should be close to 2 seconds
	if result > 3*time.Second {
		t.Errorf("expected ~2 second delay, got %v", result)
	}
}

func TestParseRetryAfter_ExceedsMaxDuration(t *testing.T) {
	veryFutureTime := time.Now().Add(10 * time.Minute)
	veryFutureHeader := veryFutureTime.Format(http.TimeFormat)

	result := parseRetryAfter(veryFutureHeader)

	if result != 0 {
		t.Errorf("expected 0 for duration > 5 minutes, got %v", result)
	}
}

func TestRetryTransport_RequestBodyReadError(t *testing.T) {
	failingReader := &failingReadCloser{}

	transport := newRetryTransport(&trackingRoundTripper{}, RetrySettings{
		MaxRetries: 1,
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, "http://example.com", failingReader)
	_, err := transport.RoundTrip(req) //nolint:bodyclose

	if err == nil {
		t.Errorf("expected error reading request body, got nil")
	}

	if !strings.Contains(err.Error(), "failed to read request body") {
		t.Errorf("expected error message containing 'failed to read request body', got %q", err.Error())
	}
}

type failingReadCloser struct{}

func (f *failingReadCloser) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func (f *failingReadCloser) Close() error {
	return nil
}

func TestRetrySettings_ApplyDefaults_AllZero(t *testing.T) {
	settings := RetrySettings{
		MaxRetries:           0,
		InitialInterval:      0,
		MaxInterval:          0,
		Multiplier:           0,
		RetriableStatusCodes: nil,
	}

	applied := settings.applyDefaults()

	if applied.MaxRetries != 3 {
		t.Errorf("expected MaxRetries default 3, got %d", applied.MaxRetries)
	}

	if applied.InitialInterval == 0 {
		t.Errorf("expected InitialInterval to have default value, got 0")
	}

	if applied.MaxInterval == 0 {
		t.Errorf("expected MaxInterval to have default value, got 0")
	}

	if applied.Multiplier == 0 {
		t.Errorf("expected Multiplier to have default value, got 0")
	}

	if len(applied.RetriableStatusCodes) == 0 {
		t.Errorf("expected RetriableStatusCodes to have defaults, got empty slice")
	}

	// Verify defaults match expected values
	if !slices.Contains(applied.RetriableStatusCodes, 429) {
		t.Errorf("expected 429 in default retriable codes")
	}
	if !slices.Contains(applied.RetriableStatusCodes, 502) {
		t.Errorf("expected 502 in default retriable codes")
	}
	if !slices.Contains(applied.RetriableStatusCodes, 503) {
		t.Errorf("expected 503 in default retriable codes")
	}
	if !slices.Contains(applied.RetriableStatusCodes, 504) {
		t.Errorf("expected 504 in default retriable codes")
	}
}
