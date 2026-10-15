package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// A smart key's primary group is not its selected request destination. Reject
// single native groups immediately, but let multi-group resolution finish first.
func rejectNonNativeSystemOneBeforeResolution(c *gin.Context, key *service.APIKey) bool {
	if key != nil && key.UsesRequestTargetPlatform() && c.Request != nil {
		if _, resolved := service.ResolvedTargetPlatformFromContext(c.Request.Context()); !resolved {
			return false
		}
	}
	return rejectNonNativeSystemOne(c, key)
}

// All protocol aliases share this boundary after composite resolution, before
// any scheduler or converter can receive a native-only credential.
func rejectNonNativeSystemOne(c *gin.Context, key *service.APIKey) bool {
	if c.Request == nil || key == nil || key.Group == nil {
		return false
	}
	platform := key.Group.Platform
	if target, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
		platform = target
	}
	if platform != service.PlatformTypeSafe {
		return false
	}
	path := c.Request.URL.Path
	if path == "/v1/systemone" || (c.Request.Method == http.MethodGet && (path == "/v1/usage" || path == "/v1/models" || path == "/models" || strings.HasPrefix(path, "/v1/models/") || strings.HasPrefix(path, "/models/"))) {
		return false
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"type": "not_found_error", "message": "TypeSafe models require POST /v1/systemone"}})
	return true
}
