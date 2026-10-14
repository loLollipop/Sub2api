package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *GatewayHandler) pinnedOpenAIModels(c *gin.Context, group *service.Group) {
	if c.Request.Context().Err() != nil {
		return
	}
	if h.openAIGatewayService == nil {
		writeOpenAIModelsError(c, http.StatusInternalServerError, "api_error", "OpenAI model discovery is not configured")
		return
	}
	etag := c.GetHeader("If-None-Match")
	if key, ok := middleware2.GetAPIKeyFromContext(c); ok && service.NormalizeModelScope(key.ModelScope) != service.ModelScopeAll {
		etag = ""
	}
	if c.Param("model") != "" {
		etag = "" // A collection ETag cannot validate a single-model representation.
	}
	response, account, err := h.openAIGatewayService.FetchPinnedOpenAIModelsList(
		c.Request.Context(), group, h.maxAccountSwitches, etag,
	)
	if c.Request.Context().Err() != nil {
		return
	}
	if err != nil {
		if errors.Is(err, service.ErrNoPinnedCodexModelsAccounts) {
			writeOpenAIModelsError(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI model discovery accounts")
			return
		}
		writeOpenAIModelsError(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)
	writeOpenAIModelsResponse(c, response, func(models []string) []string {
		key, _ := middleware2.GetAPIKeyFromContext(c)
		models = h.openAIGatewayService.FilterModelsForKeyScope(c.Request.Context(), &group.ID, key.ModelScope, models)
		return service.FilterModelsForAccountScope(key.ModelScope, models, account)
	})
}

func writeOpenAIModelsError(c *gin.Context, status int, errorType, message string) {
	c.JSON(status, gin.H{"error": gin.H{"type": errorType, "message": message}})
}

func writeOpenAIModelsResponse(c *gin.Context, manifest *service.OpenAIModelsResponse, filters ...func([]string) []string) {
	if key, ok := middleware2.GetAPIKeyFromContext(c); ok && service.NormalizeModelScope(key.ModelScope) != service.ModelScopeAll {
		body, err := filterModelCatalogueForScope(manifest.Body, key.ModelScope, filters...)
		if err != nil {
			writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid model catalogue")
			return
		}
		// Never mutate the shared manifest cache or reuse its unfiltered ETag.
		copyManifest := *manifest
		copyManifest.Body = body
		copyManifest.ETag = service.CodexModelsManifestETag(body)
		copyManifest.NotModified = c.Param("model") == "" && service.CodexModelsManifestETagMatches(c.GetHeader("If-None-Match"), copyManifest.ETag)
		manifest = &copyManifest
	}
	if c.Param("model") != "" {
		writeRetrievedModel(c, manifest.Body)
		return
	}
	if manifest.ETag != "" {
		c.Header("ETag", manifest.ETag)
	}
	if manifest.NotModified {
		c.Status(http.StatusNotModified)
		c.Writer.WriteHeaderNow()
		return
	}
	c.Data(http.StatusOK, "application/json", manifest.Body)
}

// Both discovery endpoints consume the same final catalogue, after group/platform
// selection and allowlist filtering. Preserve every field on the selected entry.
func writeModelsListResponse(c *gin.Context, models any) {
	response := gin.H{"object": "list", "data": models}
	if c.Param("model") == "" {
		c.JSON(http.StatusOK, response)
		return
	}
	body, err := json.Marshal(response)
	if err != nil {
		writeOpenAIModelsError(c, http.StatusInternalServerError, "api_error", "Failed to encode model catalogue")
		return
	}
	writeRetrievedModel(c, body)
}

func writeRetrievedModel(c *gin.Context, body []byte) {
	if key, ok := middleware2.GetAPIKeyFromContext(c); ok && !key.AllowsModelScope(c.Param("model")) {
		writeOpenAIModelsError(c, http.StatusNotFound, "model_not_found", "Model is not available for this API key")
		return
	}
	var catalog struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid model catalogue")
		return
	}
	modelID := c.Param("model")
	for _, raw := range catalog.Data {
		var model map[string]json.RawMessage
		if err := json.Unmarshal(raw, &model); err != nil {
			writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid model catalogue entry")
			return
		}
		var id string
		if err := json.Unmarshal(model["id"], &id); err != nil {
			writeOpenAIModelsError(c, http.StatusBadGateway, "upstream_error", "Invalid model catalogue ID")
			return
		}
		if id == modelID {
			c.Data(http.StatusOK, "application/json", raw)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
		"type": "invalid_request_error", "code": "model_not_found", "param": "model",
		"message": fmt.Sprintf("Model %q does not exist or is not available for this group", modelID),
	}})
}

// Preserve metadata but filter every supported catalogue shape on a private
// JSON value. Wildcards are configuration patterns, never listable models.
func filterModelCatalogueForScope(body []byte, scope string, filters ...func([]string) []string) ([]byte, error) {
	if service.NormalizeModelScope(scope) == service.ModelScopeAll {
		return body, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	for _, field := range []string{"data", "models"} {
		raw, exists := envelope[field]
		if !exists {
			continue
		}
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, err
		}
		ids := make([]string, len(entries))
		for i, entry := range entries {
			var model struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
				Name string `json:"name"`
			}
			if err := json.Unmarshal(entry, &model); err != nil {
				return nil, err
			}
			id := model.ID
			if id == "" {
				id = model.Slug
			}
			if id == "" {
				id = model.Name
			}
			ids[i] = id
		}
		allowed := service.FilterModelScope(scope, ids)
		for _, filter := range filters {
			allowed = filter(allowed)
		}
		allowedIDs := make(map[string]bool, len(allowed))
		for _, id := range allowed {
			allowedIDs[id] = true
		}
		kept := make([]json.RawMessage, 0, len(entries))
		for i, entry := range entries {
			if allowedIDs[ids[i]] {
				kept = append(kept, entry)
			}
		}
		filtered, err := json.Marshal(kept)
		if err != nil {
			return nil, err
		}
		envelope[field] = filtered
	}
	return json.Marshal(envelope)
}
