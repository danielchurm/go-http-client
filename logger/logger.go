package logger

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	log "github.com/JSainsburyPLC/go-logrus-wrapper/v2"
)

type Logger struct {
	wrapped http.RoundTripper
}

func (l Logger) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.CtxInfof(req.Context(), "failed to close body: %v", err)
		}
	}(resp.Body)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for logging: %w", err)
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	request := fmt.Sprintf("%s %s", req.Method, req.URL.Path)
}
