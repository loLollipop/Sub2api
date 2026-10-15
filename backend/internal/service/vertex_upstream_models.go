package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	vertexPublisherModelsPageSize    = 300
	vertexPublisherModelsMaxPages    = 20
	vertexGooglePublisherModelPrefix = "publishers/google/models/"
)

func (s *AccountTestService) fetchVertexGeminiUpstreamModels(ctx context.Context, account *Account) ([]string, error) {
	if account.IsVertexAPIKey() {
		return nil, newUpstreamModelSyncUnsupportedError("Vertex API key accounts do not support upstream model listing", nil)
	}
	if _, err := parseVertexServiceAccountKey(account); err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Vertex service account credentials", err)
	}
	location := account.VertexLocation("")
	if _, err := buildVertexPublisherModelsURL(location, ""); err != nil {
		return nil, newUpstreamModelSyncConfigError("Invalid Vertex location", err)
	}
	if s.geminiTokenProvider == nil {
		return nil, newUpstreamModelSyncConfigError("Gemini token provider is not configured", nil)
	}
	accessToken, err := s.geminiTokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, newUpstreamModelSyncUpstreamError("Failed to get Vertex access token", err)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, newUpstreamModelSyncConfigError("No Vertex access token is available", nil)
	}

	return listVertexGeminiPublisherModels(location, func(page int, listURL string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("Invalid Vertex model list URL", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)

		body, err := s.readUpstreamModelsResponse(req, account)
		if err != nil {
			// 只有首页的 404/405 表示列表端点不可用；翻页途中的失败不触发配置模型回退。
			var syncErr *UpstreamModelSyncError
			if page > 0 && errors.As(err, &syncErr) {
				syncErr.StatusCode = 0
			}
			return nil, err
		}
		return body, nil
	})
}

func listVertexGeminiPublisherModels(location string, fetchPage func(page int, listURL string) ([]byte, error)) ([]string, error) {
	models := make([]string, 0)
	pageToken := ""
	seenTokens := make(map[string]struct{})
	for page := 0; page < vertexPublisherModelsMaxPages; page++ {
		listURL, err := buildVertexPublisherModelsURL(location, pageToken)
		if err != nil {
			return nil, newUpstreamModelSyncConfigError("Invalid Vertex location", err)
		}
		body, err := fetchPage(page, listURL)
		if err != nil {
			return nil, err
		}
		var response struct {
			PublisherModels []struct {
				Name string `json:"name"`
			} `json:"publisherModels"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, newUpstreamModelSyncUpstreamError("Upstream model list response was not valid JSON", err)
		}
		for _, entry := range response.PublisherModels {
			name := strings.TrimSpace(entry.Name)
			if !strings.HasPrefix(name, vertexGooglePublisherModelPrefix) {
				continue
			}
			modelID := strings.TrimPrefix(name, vertexGooglePublisherModelPrefix)
			if strings.HasPrefix(modelID, "gemini-") && len(modelID) > len("gemini-") && IsSafeGeminiModelPathSegment(modelID) {
				models = append(models, modelID)
			}
		}

		pageToken = strings.TrimSpace(response.NextPageToken)
		if pageToken == "" {
			if len(models) == 0 {
				return nil, newUpstreamModelSyncUpstreamError("Upstream returned no supported models", nil)
			}
			return dedupeAndSortModelIDs(models), nil
		}
		if _, seen := seenTokens[pageToken]; seen {
			return nil, newUpstreamModelSyncUpstreamError("Upstream model list pagination repeated a token", nil)
		}
		seenTokens[pageToken] = struct{}{}
	}
	return nil, newUpstreamModelSyncUpstreamError(
		"Upstream model list pagination did not finish",
		fmt.Errorf("more than %d pages", vertexPublisherModelsMaxPages),
	)
}

func buildVertexPublisherModelsURL(location, pageToken string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		location = vertexDefaultLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}
	host := location + "-aiplatform.googleapis.com"
	if location == "global" {
		host = vertexGlobalHost
	}
	query := url.Values{"pageSize": []string{strconv.Itoa(vertexPublisherModelsPageSize)}}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	return fmt.Sprintf("https://%s/v1beta1/publishers/google/models?%s", host, query.Encode()), nil
}
