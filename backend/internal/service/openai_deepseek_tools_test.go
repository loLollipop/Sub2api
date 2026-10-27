package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func deepSeekToolsTestAccount() *Account {
	return &Account{ID: 42, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "fixture-secret", "base_url": "https://api.deepseek.com/v1"}}
}

func TestDeepSeekToolsCapability(t *testing.T) {
	for _, tc := range []struct {
		name, platform, kind string
		caps                 any
		want                 bool
	}{
		{"default", PlatformDeepseek, AccountTypeAPIKey, nil, true},
		{"explicit", PlatformDeepseek, AccountTypeAPIKey, []string{"deepseek_tools"}, true},
		{"excluded", PlatformDeepseek, AccountTypeAPIKey, []string{"chat_completions"}, false},
		{"disabled", PlatformDeepseek, AccountTypeAPIKey, map[string]bool{"deepseek_tools": false}, false},
		{"wrong platform", PlatformOpenAI, AccountTypeAPIKey, []string{"deepseek_tools"}, false},
		{"wrong kind", PlatformDeepseek, AccountTypeOAuth, []string{"deepseek_tools"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Account{Platform: tc.platform, Type: tc.kind, Credentials: map[string]any{}}
			if tc.caps != nil {
				a.Credentials[openAIEndpointCapabilitiesCredentialKey] = tc.caps
			}
			require.Equal(t, tc.want, a.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDeepSeekTools))
		})
	}
}

func TestForwardDeepSeekToolsPreservesWireAndSecuresTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{DeepSeekToolsPathSearch, DeepSeekToolsPathFetch} {
		t.Run(path, func(t *testing.T) {
			a := deepSeekToolsTestAccount()
			successBody := `{"results":[{"url":"https://example.com"}]}`
			if path == DeepSeekToolsPathFetch {
				successBody = `{"content":"Article https://example.com"}`
			}
			proxyID := int64(3)
			a.ProxyID, a.Proxy = &proxyID, &Proxy{Protocol: "http", Host: "proxy.invalid", Port: 3128, Username: "u", Password: "p"}
			body := []byte(" {\n\"query\":\"q\",\"url\":\"https://example.com\",\"future\":{\"keep\":true}} ")
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"reused"}}, Body: io.NopCloser(strings.NewReader(successBody))},
				{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"reused"}}, Body: io.NopCloser(strings.NewReader(successBody))},
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			var ids []string
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				result, err := svc.ForwardDeepSeekTools(context.Background(), c, a, path, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, result.SearchCount)
				require.True(t, result.SearchOnly)
				require.Zero(t, result.WebSearchCalls)
				require.Equal(t, path, result.UpstreamEndpoint)
				label := DeepSeekToolsLabelSearch
				if path == DeepSeekToolsPathFetch {
					label = DeepSeekToolsLabelFetch
				}
				require.Equal(t, label, result.Model)
				ids = append(ids, result.RequestID)
				require.Equal(t, "https://api.deepseek.com"+path, upstream.lastReq.URL.String())
				require.Equal(t, "Bearer fixture-secret", upstream.lastReq.Header.Get("Authorization"))
				require.Equal(t, a.Proxy.URL(), upstream.lastProxyURL)
				require.Equal(t, body, upstream.lastBody)
				require.True(t, HTTPUpstreamPublicHostsOnly(upstream.lastReq.Context()))
				require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
				require.Equal(t, successBody, w.Body.String())
			}
			require.NotEqual(t, ids[0], ids[1], "a reused upstream ID must not merge independent paid calls")
		})
	}
}

func TestForwardDeepSeekToolsFailureIsUnbilledAndRedacted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{302, 400, 401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			up := &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"http://127.0.0.1/secret"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture-secret https://u:p@private.invalid"}}`))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: up}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, DeepSeekToolsPathSearch, nil)
			result, err := svc.ForwardDeepSeekTools(context.Background(), c, deepSeekToolsTestAccount(), DeepSeekToolsPathSearch, []byte(`{"query":"q"}`))
			require.Nil(t, result)
			require.NotContains(t, w.Body.String(), "fixture-secret")
			require.Empty(t, w.Header().Get("Location"))
			if err != nil {
				require.NotContains(t, err.Error(), "fixture-secret")
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.NotContains(t, string(failover.ResponseBody), "fixture-secret")
			}
		})
	}
	t.Run("transport", func(t *testing.T) {
		up := &httpUpstreamRecorder{err: errors.New("https://u:fixture-secret@private.invalid")}
		svc := &OpenAIGatewayService{httpUpstream: up}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		result, err := svc.ForwardDeepSeekTools(context.Background(), c, deepSeekToolsTestAccount(), DeepSeekToolsPathSearch, nil)
		require.Nil(t, result)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "fixture-secret")
	})
	t.Run("URL policy", func(t *testing.T) {
		a := deepSeekToolsTestAccount()
		a.Credentials["base_url"] = "http://private.invalid"
		svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{}}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		result, err := svc.ForwardDeepSeekTools(context.Background(), c, a, DeepSeekToolsPathSearch, nil)
		require.Nil(t, result)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private.invalid")
	})
}

func TestDeepSeekToolsReservationEqualsSettlement(t *testing.T) {
	for _, tc := range []struct {
		price      *float64
		multiplier float64
		userRate   *float64
	}{
		{nil, 1.7, nil}, {floatPtr(5), 1.7, nil}, {floatPtr(13), 1.7, nil}, {floatPtr(0), 1.7, nil},
		{floatPtr(5), 0.1248, nil}, {floatPtr(5), 0, nil}, {floatPtr(5), 3, nil},
		{floatPtr(5), 3, floatPtr(0)}, {floatPtr(5), 3, floatPtr(0.1248)}, {floatPtr(5), 0.1248, floatPtr(3)},
	} {
		for _, label := range []string{DeepSeekToolsLabelSearch, DeepSeekToolsLabelFetch} {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			users := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(logs, users, &openAIRecordUsageSubRepoStub{}, nil)
			svc.userGroupRateResolver = newUserGroupRateResolver(&userGroupRateResolverRepoStub{rate: tc.userRate}, nil, time.Minute, nil, "service.test")
			gid := int64(8)
			key := &APIKey{ID: 2, UserID: 1, User: &User{ID: 1}, GroupID: &gid, Group: &Group{ID: gid, Platform: PlatformDeepseek, RateMultiplier: tc.multiplier, SearchPricePer1k: tc.price}}
			estimate, priced := (DeepSeekToolsReservationEstimator{Gateway: svc}).EstimateInflightReservation(context.Background(), key, InflightEstimateRequest{Model: label, SearchCalls: 1})
			require.True(t, priced)
			ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "reused-client-id")
			result := &OpenAIForwardResult{RequestID: label + ":unique-money-event", Model: label, SearchOnly: true, SearchCount: 1, Duration: time.Second}
			err := svc.RecordUsage(ctx, &OpenAIRecordUsageInput{Result: result, APIKey: key, User: key.User, Account: deepSeekToolsTestAccount()})
			require.NoError(t, err)
			require.NotNil(t, logs.lastLog)
			require.InDelta(t, estimate, logs.lastLog.ActualCost, 1e-12)
			require.Equal(t, result.RequestID, logs.lastLog.RequestID)
			require.Equal(t, label, logs.lastLog.Model)
			require.Equal(t, 1.0, logs.lastLog.RateMultiplier)
			expected := svc.billingService.CalculateSearchCost(1, tc.price, 1)
			require.InDelta(t, expected.ActualCost, logs.lastLog.ActualCost, 1e-12)
		}
	}
}

func TestDeepSeekToolsSearchOnlyDoesNotMaskOrdinaryTokenPricingFailure(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	result := &OpenAIForwardResult{Model: "unpriced-model-for-tools-regression", SearchCount: 1}
	_, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{}, []string{result.Model}, 1, 1, 1, 1, UsageTokens{}, "", nil, time.Time{})
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
	result.SearchOnly = true
	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, &APIKey{}, []string{result.Model}, 1, 1, 1, 1, UsageTokens{}, "", nil, time.Time{})
	require.NoError(t, err)
	require.InDelta(t, 0.005, cost.ActualCost, 1e-12)
}

func TestDeepSeekToolsSearchOnlyUsesFixedPerThousandPrice(t *testing.T) {
	svc := &OpenAIGatewayService{billingService: NewBillingService(&config.Config{}, nil)}
	apiKey := &APIKey{Group: &Group{Platform: PlatformDeepseek, SearchPricePer1k: floatPtr(5), WebSearchPricePerCall: floatPtr(99)}}
	// Standalone tools must price before token lookup, even with nonzero tokens
	// and an unpriced model. Fixed tool fees ignore model and base multipliers.
	result := &OpenAIForwardResult{Model: "unpriced-model-for-tools-regression", SearchOnly: true, SearchCount: 2}
	cost, err := svc.calculateOpenAIRecordUsageCost(context.Background(), result, apiKey, []string{result.Model}, 3, 1, 1, 0.15, UsageTokens{InputTokens: 100, OutputTokens: 10}, "", nil, time.Time{})
	require.NoError(t, err)
	require.InDelta(t, 0.01, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
}

func TestForwardDeepSeekToolsOnlyBillsActualSuccessfulResults(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		status           int
		billable         bool
	}{
		{"search", DeepSeekToolsPathSearch, `{"results":[{"url":"https://example.com"}]}`, 200, true},
		{"fetch", DeepSeekToolsPathFetch, `{"content":"article https://example.com"}`, 200, true},
		{"empty search", DeepSeekToolsPathSearch, `{"results":[]}`, 200, false},
		{"empty fetch", DeepSeekToolsPathFetch, `{"content":"  "}`, 200, false},
		{"unknown shape", DeepSeekToolsPathSearch, `{"future":"value"}`, 200, false},
		{"invalid JSON", DeepSeekToolsPathSearch, `{"results":[`, 200, false},
		{"created search", DeepSeekToolsPathSearch, `{"results":["ok"]}`, 201, false},
		{"no content", DeepSeekToolsPathFetch, ``, 204, false},
		{"error with results", DeepSeekToolsPathSearch, `{"error":{"message":"failed"},"results":["stale"]}`, 200, false},
		{"failed status", DeepSeekToolsPathFetch, `{"status":"failed","content":"stale"}`, 200, false},
		{"nested failed search", DeepSeekToolsPathSearch, `{"response":{"status":"failed"},"results":["stale"]}`, 200, false},
		{"nested failed fetch", DeepSeekToolsPathFetch, `{"response":{"status":"failed"},"content":"stale"}`, 200, false},
		{"duplicate error", DeepSeekToolsPathSearch, `{"error":null,"error":{"message":"failed"},"results":[{}]}`, 200, false},
		{"duplicate status", DeepSeekToolsPathSearch, `{"status":"completed","status":"failed","results":[{}]}`, 200, false},
		{"duplicate response", DeepSeekToolsPathFetch, `{"response":{},"response":{"status":"failed"},"content":"stale"}`, 200, false},
		{"duplicate nested status", DeepSeekToolsPathFetch, `{"response":{"status":"completed","status":"failed"},"content":"stale"}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
			result, err := svc.ForwardDeepSeekTools(t.Context(), c, deepSeekToolsTestAccount(), tc.path, nil)
			require.NoError(t, err)
			require.Equal(t, tc.billable, result != nil)
			require.Equal(t, tc.status, w.Code)
			if tc.billable {
				require.Equal(t, tc.body, w.Body.String(), "successful source URLs and tool output remain unchanged")
			}
		})
	}
}
