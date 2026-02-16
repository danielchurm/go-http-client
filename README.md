# go-http-client

A standardized HTTP client package for microservices with built-in observability, retries, and circuit breakers.

## Purpose

This package provides a team-standard HTTP client that ensures all services have:
- **Mandatory timeouts** - Prevents hanging requests
- **New Relic instrumentation** - Automatic observability (enabled by default)
- **Optional retries** - Configurable exponential backoff for transient failures
- **Optional circuit breakers** - Per-endpoint failure protection
- **Header management** - Standardized static and context-based header injection
- **Connection pooling** - Configurable TCP connection reuse for improved performance

Use this client for all external HTTP calls to maintain consistency across microservices.

## Installation
```bash
go get github.com/sainsburys-tech/go-http-client
```

## Contents

- [Basic Usage](#basic-usage)
- [Retries](#retries)
  - [Basic Retry Configuration](#basic-retry-configuration)
  - [Custom Retry Configuration](#custom-retry-configuration)
  - [What Gets Retried](#what-gets-retried)
  - [Retry-After Header Support](#retry-after-header-support)
  - [Retry Timing and Timeouts](#retry-timing-and-timeouts)
- [Connection Pooling](#connection-pooling)
  - [Default Pooling Behavior](#default-pooling-behavior)
  - [Custom Pooling Configuration](#custom-pooling-configuration)
  - [When to Customize Pooling](#when-to-customize-pooling)
  - [Advanced Timeout Settings](#advanced-timeout-settings)
  - [Disabling Keep-Alive](#disabling-keep-alive-not-recommended)
  - [Disabling Compression](#disabling-compression-not-recommended)
  - [Limiting Response Header Size](#limiting-response-header-size)
- [Headers](#headers)
  - [Static Headers](#static-headers)
  - [Context Headers](#context-headers)
  - [Combining Static and Context Headers](#combining-static-and-context-headers)
  - [Important Notes on Headers](#important-notes-on-headers)
- [Circuit Breakers](#circuit-breakers)
  - [Step 1: Define Circuit Breaker Keys](#step-1-define-circuit-breaker-keys)
  - [Step 2: Configure Circuit Breakers](#step-2-configure-circuit-breakers)
  - [Step 3: Use Circuit Breakers in Application Code](#step-3-use-circuit-breakers-in-application-code)
  - [Understanding ShouldTrip](#understanding-shouldtrip)
  - [Circuit Breaker States](#circuit-breaker-states)
  - [Adding Circuit Breakers](#adding-circuit-breakers)
- [Combining Retries and Circuit Breakers](#combining-retries-and-circuit-breakers)
- [Advanced Options](#advanced-options)
- [Complete Example](#complete-example)
- [Best Practices](#best-practices)
- [FAQ](#faq)



## Basic Usage

**Key points:**
- **Timeout is mandatory** - Must be greater than 0. All retries and backoff happen within this timeout.
- **New Relic enabled by default** - Automatic observability for all requests. Only disable with `WithoutNewRelic()` in tests.
- **Retries happen at transport level** - Before circuit breakers see them. Each retry increments the circuit breaker's failure count.
- **With `MaxRetries: 3`, one logical request can add 4 failures to the breaker** - (initial + 3 retries). Set your circuit breaker thresholds accordingly: `(MaxRetries + 1) * desiredlogicalfailures`
- **Only idempotent methods are retried** - GET, PUT, DELETE, HEAD, OPTIONS, TRACE. POST and PATCH are never retried to prevent duplicate operations.
- **Non-idempotent status codes are validated** - Retry settings are validated at client initialization. Invalid configurations fail fast with clear error messages.
- **Connection pooling is configured via options** - Control max idle connections, per-host limits, and idle timeout for better resource management.

### Creating a Client

The simplest client requires only a timeout:
```go
import (
    "time"
    httpclient "github.com/sainsburys-tech/go-http-client"
)

func main() {
    client, err := httpclient.NewClient(30 * time.Second)
    if err != nil {
        panic(err)
    }
    
    // Use with oapi-codegen generated clients
    apiClient, _ := api.NewClientWithResponses(
        "https://api.example.com",
        api.WithHTTPClient(client.Client),
    )
}
```

## Retries

### Basic Retry Configuration

Add automatic retries with sensible defaults:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithRetries(httpclient.RetrySettings{}), // uses all defaults
)
```

**Default retry behavior:**
- **MaxRetries:** 3 attempts
- **InitialInterval:** 500ms
- **MaxInterval:** 60s
- **Multiplier:** 1.5 (exponential backoff)
- **RetriableStatusCodes:** [429, 502, 503, 504]

### Custom Retry Configuration

Override any defaults:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithRetries(httpclient.RetrySettings{
        MaxRetries:           5,
        InitialInterval:      1 * time.Second,
        MaxInterval:          10 * time.Second,
        Multiplier:           2.0,
        RetriableStatusCodes: []int{503, 504}, // Only retry these
    }),
)
```

### What Gets Retried

**Retried automatically:**
- Network errors (connection refused, timeouts, DNS failures)
- Configured status codes (default: 429, 502, 503, 504)
- Only idempotent methods: GET, PUT, DELETE, HEAD, OPTIONS, TRACE

**NOT retried:**
- POST and PATCH requests (not idempotent - could duplicate operations)
- 2xx success responses
- 4xx client errors (except 429 if configured)

### Retry-After Header Support

The retry mechanism respects `Retry-After` headers from servers:

- When a server returns 429 or 503 with a `Retry-After` header, the client waits for the specified duration
- Supports both formats: seconds (`Retry-After: 120`) and HTTP dates (`Retry-After: Wed, 21 Oct 2015 07:28:00 GMT`)
- Maximum wait time is capped at 5 minutes for safety

### Retry Timing and Timeouts

**Important:** The client timeout includes all retry attempts. Configure your retry settings to ensure they complete within the timeout:
```go
// Example: 30s timeout with retries
client, err := httpclient.NewClient(
    30 * time.Second, // Total time for request + all retries
    httpclient.WithRetries(httpclient.RetrySettings{
        MaxRetries:      3,
        InitialInterval: 500 * time.Millisecond,
        MaxInterval:     5 * time.Second,
        Multiplier:      2.0,
    }),
)
```

The client validates that worst-case retry backoff time is less than the timeout and returns an error during initialization if misconfigured.

## Connection Pooling

Connection pooling reuses TCP connections across multiple requests, reducing latency and improving throughput. This client configures connection pooling by cloning Go's default transport and allowing customization.

### Default Pooling Behavior

By default, the client uses Go's standard library defaults:
- **MaxIdleConns:** 100 idle connections total
- **MaxIdleConnsPerHost:** 2 idle connections per host
- **IdleConnTimeout:** 90 seconds (connections closed if idle longer)
- **MaxConnsPerHost:** Unlimited

These defaults are fine for most services.

### Custom Pooling Configuration

Override defaults for high-concurrency scenarios:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        MaxIdleConns:        50,  // Total idle connections across all hosts
        MaxIdleConnsPerHost: 10,  // Idle connections per host
        MaxConnsPerHost:     20,  // Total connections per host (0 = unlimited)
        IdleConnTimeout:     30 * time.Second,
    }),
)
```

### When to Customize Pooling

**Increase pool size if:**
- Your service makes many concurrent requests
- You're calling multiple upstream services
- You see "connection reset" errors under load

**Example for high-concurrency service:**
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        MaxIdleConns:        200, // Accommodate many concurrent requests
        MaxIdleConnsPerHost: 50,
        MaxConnsPerHost:     100,
        IdleConnTimeout:     60 * time.Second,
    }),
)
```

### Advanced Timeout Settings

Fine-grained control over specific timeout scenarios:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        // Connection timeouts
        TLSHandshakeTimeout:   10 * time.Second, // How long to negotiate TLS
        ResponseHeaderTimeout: 15 * time.Second, // Max wait for response headers
        ExpectContinueTimeout: 1 * time.Second,  // Timeout for 100-continue
        
        // Other settings
        IdleConnTimeout: 30 * time.Second,
    }),
)
```

### Disabling Keep-Alive (not recommended)

In rare cases, you might need to disable HTTP keep-alives:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        DisableKeepAlives: true, // Force new connection per request
    }),
)
```

**Warning:** Disabling keep-alives significantly increases latency and resource usage. Only use if absolutely necessary.

### Disabling Compression (not recommended)

The client respects HTTP compression by default. In rare cases where you need to disable it:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        DisableCompression: true,
    }),
)
```

### Limiting Response Header Size

Protect against malicious servers sending huge headers:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithConnectionPool(httpclient.PoolSettings{
        MaxResponseHeaderBytes: 16384, // 16 KB (default is 10 MB)
    }),
)
```

## Headers

The client provides standardized header management for both static headers (same for all requests) and context-based headers (vary per request).

### Static Headers

Add headers that are sent with every request:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithHeaders(httpclient.HeaderSettings{
        StaticHeaders: map[string]string{
            "X-API-Key":      "secret-key-123",
            "X-Client-Name":  "order-service",
            "X-Client-Version": "1.2.3",
        },
    }),
)
```

### Context Headers

Add headers based on request context (e.g., request ID, user ID, tenant ID):
```go
type ctxKey string

const (
    RequestIDKey ctxKey = "request-id"
    UserIDKey    ctxKey = "user-id"
    TenantIDKey  ctxKey = "tenant-id"
)

client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithHeaders(httpclient.HeaderSettings{
        ContextHeaders: map[string]any{
            "X-Request-ID": RequestIDKey,
            "X-User-ID":    UserIDKey,
            "X-Tenant-ID":  TenantIDKey,
        },
    }),
)

// Use with context
ctx := context.WithValue(context.Background(), RequestIDKey, "req-abc123")
ctx = context.WithValue(ctx, UserIDKey, "user-456")
ctx = context.WithValue(ctx, TenantIDKey, "tenant-789")

req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.example.com/users", nil)
resp, err := client.Do(req)
// Request will have: X-Request-ID: req-abc123, X-User-ID: user-456, X-Tenant-ID: tenant-789
```

### Combining Static and Context Headers

Both types work together:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithHeaders(httpclient.HeaderSettings{
        StaticHeaders: map[string]string{
            "X-API-Key":    "my-key",
            "X-App-Name":   "order-service",
        },
        ContextHeaders: map[string]any{
            "X-Request-ID": RequestIDKey,
            "X-User-ID":    UserIDKey,
        },
    }),
)

// This request will have all 4 headers
ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
ctx = context.WithValue(ctx, UserIDKey, "user-456")
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, _ := client.Do(req)
```

### Important Notes on Headers

- **Static headers are never overwritten** - If your request already has a header, it won't be overwritten
- **Context headers only added if key exists in context** - Missing context values are silently skipped
- **All context values are converted to strings** - Using `fmt.Sprint()`, since all HTTP headers are strings. Pass integers, UUIDs, booleans, etc. and they'll be automatically converted.
- **Existing headers take precedence** - This prevents accidentally overwriting important headers

## Circuit Breakers

Circuit breakers provide per-endpoint failure protection. When an endpoint fails repeatedly, the circuit breaker "opens" and fails fast without making requests, giving the downstream service time to recover.

### Step 1: Define Circuit Breaker Keys

Create strongly-typed constants for your circuit breakers:
```go
package main

import httpclient "github.com/sainsburys-tech/go-http-client"

const (
    BreakerUsersGet       httpclient.CircuitBreakerKey = "users:get"
    BreakerOrdersCreate   httpclient.CircuitBreakerKey = "orders:create"
    BreakerPaymentProcess httpclient.CircuitBreakerKey = "payment:process"
)
```

### Step 2: Configure Circuit Breakers
```go
import (
    "time"
    "github.com/sony/gobreaker/v2"
    httpclient "github.com/sainsburys-tech/go-http-client"
)

client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithCircuitBreakers([]httpclient.CircuitBreakerSettings{
        {
            Key: BreakerUsersGet,
            Settings: gobreaker.Settings{
                Name:        string(BreakerUsersGet),
                MaxRequests: 10,                    // Max requests allowed in half-open state
                Interval:    60 * time.Second,      // Period to clear internal counts
                Timeout:     30 * time.Second,      // How long to stay open before trying half-open
                ReadyToTrip: func(counts gobreaker.Counts) bool {
                    return counts.ConsecutiveFailures >= 5
                },
            },
            ShouldTrip: func(statusCode int) bool {
                return statusCode >= 500  // Only trip on server errors
            },
        },
    }),
)
```

### Step 3: Use Circuit Breakers in Application Code

Circuit breakers are used at the **application level** with oapi-codegen clients.

#### Recommended: Using ExecuteWithBreaker Helper

The `ExecuteWithBreaker` helper reduces boilerplate by automatically handling the `ShouldTrip` logic:

```go
import (
    "context"
    "errors"
    "github.com/sony/gobreaker/v2"
    httpclient "github.com/sainsburys-tech/go-http-client"
)

func (s *Service) GetUser(ctx context.Context, userID string) (*User, error) {
    var oapiResp *api.GetUserResponse
    
    // Execute request through circuit breaker with automatic ShouldTrip handling
    _, err := s.httpClient.ExecuteWithBreaker(BreakerUsersGet, func() (*http.Response, error) {
        var err error
        oapiResp, err = s.apiClient.GetUserWithResponse(ctx, userID)
        if err != nil {
            return nil, err
        }
        return oapiResp.HTTPResponse, nil
    })
    
    // Handle errors (network, timeout, or circuit breaker open)
    if err != nil {
        if errors.Is(err, gobreaker.ErrOpenState) {
            return nil, fmt.Errorf("user service circuit breaker is open: %w", err)
        }
        if !errors.Is(err, httpclient.ErrBadResponse) {
            return nil, fmt.Errorf("failed to get user: %w", err)
        }
    }
    
    // Work with typed response
    if oapiResp.JSON500 != nil {
        return nil, fmt.Errorf("server error: %s", oapiResp.JSON500.Message)
    }
    
    if oapiResp.JSON200 == nil {
        return nil, fmt.Errorf("unexpected response status: %d", oapiResp.StatusCode())
    }
    
    return oapiResp.JSON200, nil
}
```

#### Alternative: Manual Circuit Breaker Control

For more control, you can manually execute the circuit breaker and handle the `ShouldTrip` logic:

```go
func (s *Service) GetUser(ctx context.Context, userID string) (*User, error) {
    var oapiResp *api.GetUserResponse
    
    // Get the circuit breaker for this endpoint
    cb := s.httpClient.GetBreaker(BreakerUsersGet)
    
    // Execute request through circuit breaker
    _, err := cb.Execute(func() (*http.Response, error) {
        var err error
        oapiResp, err = s.apiClient.GetUserWithResponse(ctx, userID)
        if err != nil {
            return nil, err
        }
        
        // Check if response should trip the circuit breaker
        if s.httpClient.ShouldTrip(BreakerUsersGet, oapiResp.StatusCode()) {
            return oapiResp.HTTPResponse, httpclient.ErrBadResponse
        }
        
        return oapiResp.HTTPResponse, nil
    })
    
    // Handle errors (network, timeout, or circuit breaker open)
    if err != nil && !errors.Is(err, httpclient.ErrBadResponse) {
        return nil, fmt.Errorf("failed to get user: %w", err)
    }
    
    // Work with typed response
    if oapiResp.JSON500 != nil {
        return nil, fmt.Errorf("server error: %s", oapiResp.JSON500.Message)
    }
    
    if oapiResp.JSON200 == nil {
        return nil, fmt.Errorf("unexpected response status: %d", oapiResp.StatusCode())
    }
    
    return oapiResp.JSON200, nil
}
```

### Understanding ShouldTrip

The `ShouldTrip` function determines which HTTP status codes should count as failures for the circuit breaker:
```go
// Default behavior (if nil): only 5xx errors trip the breaker
ShouldTrip: nil  // equivalent to: func(code int) bool { return code >= 500 }

// Custom: trip on rate limits AND server errors
ShouldTrip: func(code int) bool {
    return code == 429 || code >= 500
}

// Custom: never trip on 503 (service might intentionally return this)
ShouldTrip: func(code int) bool {
    return code >= 500 && code != 503
}
```

**Important:** 4xx errors (404, 401, 403) typically shouldn't trip circuit breakers - they indicate client errors, not service health issues.

### Circuit Breaker States

Circuit breakers have three states:

1. **Closed** (normal): Requests pass through. Failures are counted.
2. **Open** (tripped): Requests fail immediately without hitting the service. The breaker stays open for `Timeout` duration.
3. **Half-Open** (testing): After `Timeout`, allows `MaxRequests` through to test if service recovered. If successful, closes. If failures continue, opens again.

### Adding Circuit Breakers

Use `WithCircuitBreakers` for multiple breakers or `WithCircuitBreaker` for a single one:
```go
// Multiple breakers
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithCircuitBreakers([]httpclient.CircuitBreakerSettings{
        {Key: BreakerUsersGet, Settings: gobreaker.Settings{...}},
        {Key: BreakerOrdersCreate, Settings: gobreaker.Settings{...}},
    }),
)

// Single breaker
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithCircuitBreaker(httpclient.CircuitBreakerSettings{
        Key:      BreakerUsersGet,
        Settings: gobreaker.Settings{...},
    }),
)
```

## Combining Retries and Circuit Breakers

When using both retries and circuit breakers, be aware of their interaction:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithRetries(httpclient.RetrySettings{
        MaxRetries: 3, // Each request can retry 3 times
    }),
    httpclient.WithCircuitBreakers([]httpclient.CircuitBreakerSettings{
        {
            Key: BreakerUsersGet,
            Settings: gobreaker.Settings{
                Name: string(BreakerUsersGet),
                ReadyToTrip: func(counts gobreaker.Counts) bool {
                    // With 3 retries, each logical request = 4 attempts
                    // Set threshold accounting for retries
                    return counts.ConsecutiveFailures >= 12 // ~3 logical requests
                },
            },
        },
    }),
)
```

## Advanced Options

### Disable New Relic (not recommended)

New Relic instrumentation is enabled by default. Only disable for testing:
```go
client, err := httpclient.NewClient(
    30 * time.Second,
    httpclient.WithoutNewRelic(),
)
```

## Complete Example
```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"
    
    "github.com/sony/gobreaker/v2"
    httpclient "github.com/sainsburys-tech/go-http-client"
    "github.com/sainsburys-tech/api" // your oapi-codegen client
)

// Define circuit breaker keys as constants
const (
    BreakerUsersGet httpclient.CircuitBreakerKey = "users:get"
)

type UserService struct {
    httpClient *httpclient.HTTPClient
    apiClient  *api.ClientWithResponses
}

func NewUserService() (*UserService, error) {
    // Create HTTP client with retries and circuit breakers
    httpClient, err := httpclient.NewClient(
        30 * time.Second,
        httpclient.WithRetries(httpclient.RetrySettings{
            MaxRetries:      3,
            InitialInterval: 500 * time.Millisecond,
            MaxInterval:     5 * time.Second,
            Multiplier:      2.0,
        }),
        httpclient.WithCircuitBreakers([]httpclient.CircuitBreakerSettings{
            {
                Key: BreakerUsersGet,
                Settings: gobreaker.Settings{
                    Name:        string(BreakerUsersGet),
                    MaxRequests: 10,
                    Interval:    60 * time.Second,
                    Timeout:     30 * time.Second,
                    ReadyToTrip: func(counts gobreaker.Counts) bool {
                        // Account for retries: 4 attempts × 3 logical requests = 12
                        return counts.ConsecutiveFailures >= 12
                    },
                },
                ShouldTrip: func(code int) bool {
                    return code >= 500
                },
            },
        }),
    )
    if err != nil {
        return nil, err
    }
    
    // Create oapi-codegen client
    apiClient, err := api.NewClientWithResponses(
        "https://api.example.com",
        api.WithHTTPClient(httpClient.Client),
    )
    if err != nil {
        return nil, err
    }
    
    return &UserService{
        httpClient: httpClient,
        apiClient:  apiClient,
    }, nil
}

func (s *UserService) GetUser(ctx context.Context, userID string) (*api.User, error) {
    var oapiResp *api.GetUserResponse
    
    // Use ExecuteWithBreaker helper to reduce boilerplate
    _, err := s.httpClient.ExecuteWithBreaker(BreakerUsersGet, func() (*http.Response, error) {
        var err error
        oapiResp, err = s.apiClient.GetUserWithResponse(ctx, userID)
        if err != nil {
            return nil, err
        }
        return oapiResp.HTTPResponse, nil
    })
    
    if err != nil {
        if errors.Is(err, gobreaker.ErrOpenState) {
            return nil, fmt.Errorf("user service unavailable: %w", err)
        }
        if !errors.Is(err, httpclient.ErrBadResponse) {
            return nil, fmt.Errorf("failed to get user: %w", err)
        }
    }
    
    if oapiResp.JSON200 == nil {
        return nil, fmt.Errorf("unexpected response: %d", oapiResp.StatusCode())
    }
    
    return oapiResp.JSON200, nil
}

func main() {
    service, err := NewUserService()
    if err != nil {
        panic(err)
    }
    
    user, err := service.GetUser(context.Background(), "123")
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("User: %+v\n", user)
}
```

## Best Practices

1. **Always set timeouts** - Never use `0` (infinite timeout)
2. **Define CB keys as constants** - Prevents typos and enables autocomplete  
3. **Use circuit breakers per-endpoint** - Different endpoints may have different failure characteristics
4. **Don't trip on 4xx errors** - These indicate client problems, not service health
5. **Consider rate limits carefully** - Decide if 429 should trip your breaker based on whether it's per-client or global
6. **Account for retries in CB thresholds** - Set `MaxFailures` to `(MaxRetries + 1) * desiredlogicalfailures`
7. **Validate retry timing** - Ensure worst-case retry time is less than client timeout
8. **Use sensible retry defaults** - Start with defaults and customize only when needed
9. **Test circuit breaker behavior** - Ensure your service degrades gracefully when breakers open

## FAQ

**Q: When should I use retries vs circuit breakers?**  
A: Use both! Retries handle transient failures (temporary network issues). Circuit breakers protect against sustained outages (service is down). They work together - retries happen first at the transport level, then circuit breakers track the overall health at the application level.

**Q: What happens when a circuit breaker opens?**  
A: Requests fail immediately with an error, without making the HTTP call. This protects the downstream service and makes your service fail fast.

**Q: Why do retries increment the circuit breaker failure count?**  
A: Because retries happen at the transport level before the circuit breaker sees the result. If all retries fail, it's a strong signal the service is unhealthy. Adjust your `ReadyToTrip` threshold to account for this: `(MaxRetries + 1) * desiredlogicalfailures`.

**Q: Can I use this with non-oapi-codegen clients?**  
A: Yes! Just pass `client.Client` to any function expecting `*http.Client`. Circuit breakers work best with oapi-codegen but can be adapted for any HTTP client.

**Q: Why is New Relic enabled by default?**  
A: Observability should be the default. Every service should report metrics. Only disable for local testing.

**Q: What if I only want to override one retry setting?**  
A: Just set that field - all others will use defaults:
```go
httpclient.WithRetries(httpclient.RetrySettings{
    MaxRetries: 5, // others use defaults
})
```

**Q: How do I know if my retry configuration will work with my timeout?**  
A: The client validates this during initialization. If worst-case retry backoff exceeds the timeout, `NewClient` returns an error. Fix by reducing `MaxRetries`/`MaxInterval` or increasing the timeout.
