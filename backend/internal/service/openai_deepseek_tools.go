package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	DeepSeekToolsPathSearch  = "/v1/tools/search"
	DeepSeekToolsPathFetch   = "/v1/tools/fetch"
	DeepSeekToolsLabelSearch = "deepseek-tools-search"
	DeepSeekToolsLabelFetch  = "deepseek-tools-fetch"
)

// DeepSeekToolsReservationEstimator prices standalone tools using the same
// per-search price and base user/group multiplier as final usage settlement.
// Channel model prices and token peak multipliers do not apply to these calls.
type DeepSeekToolsReservationEstimator struct{ Gateway *OpenAIGatewayService }

func (e DeepSeekToolsReservationEstimator) EstimateInflightReservation(ctx context.Context, key *APIKey, _ InflightEstimateRequest) (float64, bool) {
	s := e.Gateway
	if s == nil || s.billingService == nil || key == nil || key.User == nil {
		return 0, false
	}
	rate := 1.0
	if s.cfg != nil {
		rate = s.cfg.Default.RateMultiplier
	}
	if key.GroupID != nil && key.Group != nil {
		rate = s.ResolveUserGroupRateMultiplier(ctx, key.User.ID, *key.GroupID, key.Group.RateMultiplier)
	}
	cost := s.billingService.CalculateSearchCost(1, groupSearchPricePer1kFromAPIKey(key), rate)
	return cost.ActualCost, true
}

// ForwardDeepSeekTools preserves the JSON wire payload. Only a successful 2xx
// produces a billable result; account credentials never follow redirects.
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
	c.Data(resp.StatusCode, contentType, responseBody)
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
