package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	DeepSeekToolsPathSearch  = "/v1/tools/search"
	DeepSeekToolsPathFetch   = "/v1/tools/fetch"
	DeepSeekToolsLabelSearch = "deepseek-tools-search"
	DeepSeekToolsLabelFetch  = "deepseek-tools-fetch"
)

// DeepSeekToolsReservationEstimator prices standalone tools using the same
// per-search price as final usage settlement. Standalone tool fees are fixed;
// user/group multipliers and token peak multipliers do not apply to these calls.
type DeepSeekToolsReservationEstimator struct{ Gateway *OpenAIGatewayService }

func (e DeepSeekToolsReservationEstimator) EstimateInflightReservation(ctx context.Context, key *APIKey, _ InflightEstimateRequest) (float64, bool) {
	s := e.Gateway
	if s == nil || s.billingService == nil || key == nil || key.User == nil {
		return 0, false
	}
	cost := s.billingService.CalculateSearchCost(1, groupSearchPricePer1kFromAPIKey(key), 1)
	return cost.ActualCost, true
}

// ForwardDeepSeekTools preserves successful JSON wire payloads. Only HTTP 200
// with actual results is billable; account credentials never follow redirects.
func (s *OpenAIGatewayService) ForwardDeepSeekTools(ctx context.Context, c *gin.Context, account *Account, path string, body []byte) (*OpenAIForwardResult, error) {
	if s == nil || c == nil || account == nil {
		return nil, errors.New("service, context, and account are required")
	}
	if !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDeepSeekTools) {
		return nil, errors.New("account does not support DeepSeek tools")
	}
	label := DeepSeekToolsLabelSearch
	switch path {
	case DeepSeekToolsPathSearch:
	case DeepSeekToolsPathFetch:
		label = DeepSeekToolsLabelFetch
	default:
		return nil, errors.New("unsupported DeepSeek tools endpoint")
	}
	baseURL, err := s.validateUpstreamBaseURL(account.GetOpenAIFormatBaseURL())
	if err != nil {
		return nil, errors.New("invalid DeepSeek tools upstream URL")
	}
	credential := strings.TrimSpace(account.GetCNAPIKey())
	if credential == "" {
		return nil, errors.New("DeepSeek tools account has no API key")
	}
	ctx = WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(ctx))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildOpenAIEndpointURL(baseURL, path), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to build DeepSeek tools request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	start := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(start).Milliseconds())
	if err != nil {
		return nil, errors.New("DeepSeek tools upstream request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, errors.New("failed to read DeepSeek tools upstream response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// Never expose credential-bearing URLs, proxy errors, provider diagnostics,
		// or redirect locations to clients or failover logs.
		safeBody := []byte(`{"error":{"type":"upstream_error","message":"DeepSeek tools upstream request failed"}}`)
		message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(responseBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, message, responseBody) {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: safeBody}
		}
		status := resp.StatusCode
		if status < http.StatusBadRequest {
			status = http.StatusBadGateway
		}
		c.Data(status, "application/json", safeBody)
		return nil, nil
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, responseheaders.ContentType(contentType, "application/json"), sanitizeClientUpstreamErrorBody(resp.StatusCode, responseBody))
	if resp.StatusCode != http.StatusOK || !deepSeekToolsHasBillableResult(path, responseBody) {
		return nil, nil
	}
	return &OpenAIForwardResult{
		RequestID:        fmt.Sprintf("%s:%s", label, uuid.NewString()),
		UpstreamHeaders:  resp.Header,
		Model:            label,
		UpstreamEndpoint: path,
		Duration:         time.Since(start),
		SearchOnly:       true,
		SearchCount:      1,
	}, nil
}

func deepSeekToolsHasBillableResult(path string, body []byte) bool {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return false
	}
	if clientUpstreamBodyIsError(body) {
		return false
	}
	if path == DeepSeekToolsPathSearch {
		results := gjson.GetBytes(body, "results")
		return results.IsArray() && len(results.Array()) > 0
	}
	content := gjson.GetBytes(body, "content")
	return path == DeepSeekToolsPathFetch && content.Type == gjson.String && strings.TrimSpace(content.String()) != ""
}
