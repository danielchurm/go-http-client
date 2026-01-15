package circuitbreaker_test

import (
	"errors"
	"net/http"

	"github.com/JSainsburyPLC/smartshop-api-shopper-orchestrator/circuitbreaker"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sony/gobreaker/v2"

	"testing"
)

func TestCircuitBreaker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CircuitBreaker")
}

var _ = Describe("Circuit Breaker", func() {
	It("trips when status code 500 is returned", func() {
		circuitBreakerRoundTripper := circuitbreaker.NewRoundTripper(
			&testRoundTripper{StatusCode: http.StatusInternalServerError},
			circuitbreaker.Settings{
				Settings: gobreaker.Settings{ReadyToTrip: readyToTrip},
			},
		)

		resp, err := circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).ToNot(HaveOccurred(), "error returned on the first call")
		Expect(resp).ToNot(BeNil(), "no response returned")

		_, err = circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).To(MatchError(gobreaker.ErrOpenState), "did not enter open state on second call")
	})

	It("does not trip when ShouldTrip is changed", func() {
		circuitBreakerRoundTripper := circuitbreaker.NewRoundTripper(
			&testRoundTripper{StatusCode: http.StatusInternalServerError},
			circuitbreaker.Settings{
				Settings:   gobreaker.Settings{ReadyToTrip: readyToTrip},
				ShouldTrip: func(int) bool { return false },
			},
		)

		resp, err := circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).ToNot(HaveOccurred(), "error returned on the first call")
		Expect(resp).ToNot(BeNil(), "no response returned")

		_, err = circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).ToNot(HaveOccurred(), "circuitbreaker should not have been tripped")
	})

	It("trips when a non-HTTP error occurs", func() {
		expectedError := errors.New("oh no")

		circuitBreakerRoundTripper := circuitbreaker.NewRoundTripper(
			&testRoundTripper{Error: expectedError},
			circuitbreaker.Settings{
				Settings: gobreaker.Settings{ReadyToTrip: readyToTrip},
			},
		)

		resp, err := circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).To(MatchError(expectedError), "error not returned")
		Expect(resp).To(BeNil(), "resp should be nil")

		_, err = circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).To(MatchError(gobreaker.ErrOpenState), "did not enter open state on second call")
	})

	It("trips after multiple consecutive failures", func() {
		consecutiveFailuresAllowed := 3
		circuitBreakerRoundTripper := circuitbreaker.NewRoundTripper(
			&testRoundTripper{StatusCode: http.StatusInternalServerError},
			circuitbreaker.Settings{
				Settings: gobreaker.Settings{ReadyToTrip: func(counts gobreaker.Counts) bool {
					return counts.ConsecutiveFailures >= uint32(consecutiveFailuresAllowed)
				}},
			},
		)

		for range consecutiveFailuresAllowed {
			resp, err := circuitBreakerRoundTripper.RoundTrip(nil)
			Expect(err).ToNot(HaveOccurred(), "error returned during allowed consecutive failures")
			Expect(resp).ToNot(BeNil(), "no response returned")
		}

		_, err := circuitBreakerRoundTripper.RoundTrip(nil)
		Expect(err).To(MatchError(gobreaker.ErrOpenState), "did not enter open state on second call")
	})
})

type testRoundTripper struct {
	StatusCode int
	Error      error
}

func (rt testRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if rt.Error != nil {
		return nil, rt.Error
	}
	return &http.Response{StatusCode: rt.StatusCode}, nil
}

// enter open state after 1 error
func readyToTrip(gobreaker.Counts) bool { return true }
