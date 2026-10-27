package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/util/clientprivacy"
)

// sanitizeClientUpstreamErrorBody is an egress-only boundary. Classifiers,
// retry decisions and billing must continue to receive the original bytes.
// Successful model text, tool results and search source URLs are untouched.
func sanitizeClientUpstreamErrorBody(status int, body []byte) []byte {
	isError := status < http.StatusOK || status >= http.StatusMultipleChoices || clientUpstreamBodyIsError(body)
	if !isError {
		if bodyHasSSEFraming(body) {
			return []byte(clientprivacy.SSE(string(body)))
		}
		return body
	}
	if bodyHasSSEFraming(body) {
		return []byte(clientprivacy.ErrorSSE(string(body)))
	}
	return clientprivacy.JSON(body)
}

// SanitizeClientUpstreamErrorBody protects handler-owned response writes.
// Do not replace internal result bodies used for classification or accounting.
func SanitizeClientUpstreamErrorBody(status int, body []byte) []byte {
	return sanitizeClientUpstreamErrorBody(status, body)
}

func clientUpstreamBodyIsError(body []byte) bool {
	return clientprivacy.IsErrorJSON(body)
}
