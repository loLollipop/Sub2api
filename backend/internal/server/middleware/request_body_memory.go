package middleware

import (
	"errors"
	"net/http"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

// RejectRequestBodyReadError handles early ingress termination before the route
// handler runs. Cancelled clients have no response body and an uncommitted
// response is marked 499, matching the gateway handler's cancellation path.
func RejectRequestBodyReadError(c *gin.Context, err error) bool {
	if err != nil && c != nil && c.Request != nil && c.Request.Context().Err() != nil {
		if !c.Writer.Written() {
			c.Status(499)
		}
		c.Abort()
		return true
	}
	return RejectRequestBodyMemoryOverload(c, err)
}

// RejectRequestBodyMemoryOverload keeps temporary capacity errors distinct
// from invalid JSON and the per-request 413 size limit.
func RejectRequestBodyMemoryOverload(c *gin.Context, err error) bool {
	if !errors.Is(err, pkghttputil.ErrRequestBodyMemoryBudget) {
		return false
	}
	c.Header("Retry-After", "1")
	const message = "Request body buffering capacity is exhausted; retry later"
	path := c.Request.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta"):
		GoogleErrorWriter(c, http.StatusServiceUnavailable, message)
	case strings.Contains(path, "/messages"):
		AnthropicErrorWriter(c, http.StatusServiceUnavailable, message)
	default:
		// The allowlist's OpenAI writer uses model_not_found, which would make
		// temporary memory pressure look like a permanent model rejection.
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{
			"type": "server_error", "code": "request_body_memory_exhausted", "message": message,
		}})
	}
	c.Abort()
	return true
}
