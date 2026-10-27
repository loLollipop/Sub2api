package service

import (
	"fmt"
	"net/http"
)

// AnthropicPassthroughStreamError is a failed upstream stream whose original
// error event has already been handled by the Anthropic API-key passthrough.
// It is deliberately not an UpstreamFailoverError: replaying an accepted stream
// could duplicate output and billing. Observed usage may still accompany it.
type AnthropicPassthroughStreamError struct {
	StatusCode      int
	ErrorType       string
	Message         string
	RawData         string
	ResponseWritten bool
}

func (e *AnthropicPassthroughStreamError) Error() string {
	return fmt.Sprintf("anthropic upstream stream failed: status=%d type=%s", e.StatusCode, e.ErrorType)
}

// Gin implements Flush without an error, even when the underlying HTTP writer
// exposes FlushError. Unwrap first so a failed network flush cannot acknowledge
// a terminal response that was only accepted into an intermediate buffer.
func flushAnthropicPassthroughError(writer http.ResponseWriter) error {
	for {
		if flusher, ok := writer.(interface{ FlushError() error }); ok {
			return flusher.FlushError()
		}
		if wrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter }); ok {
			writer = wrapper.Unwrap()
			continue
		}
		return http.NewResponseController(writer).Flush()
	}
}
