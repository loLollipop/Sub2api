//go:build unit

package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type emptyCCFailoverUpstream struct {
	service.HTTPUpstream
	mu        sync.Mutex
	firstBody string
	ids       []int64
	auths     []string
	paths     []string
	models    []string
	allEmpty  bool
}

func (u *emptyCCFailoverUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	u.ids = append(u.ids, id)
	u.auths = append(u.auths, req.Header.Get("Authorization"))
	u.paths = append(u.paths, req.URL.Path)
	u.models = append(u.models, gjson.GetBytes(payload, "model").String())
	u.mu.Unlock()
	body := u.firstBody
	if id == 2 && !u.allEmpty {
		body = "data: {\"id\":\"healthy\",\"model\":\"glm-5.3\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"healthy fallback\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"id\":\"healthy\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n\n"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func newCCEmptyFailoverTestHandler(t *testing.T, upstream *emptyCCFailoverUpstream) *OpenAIGatewayHandler {
	t.Helper()
	return newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts []service.Account, cfg *config.Config) {
		cfg.Security.URLAllowlist.Enabled = false
		for i := range accounts {
			accounts[i].Type = service.AccountTypeAPIKey
			accounts[i].GroupIDs = []int64{3131}
			accounts[i].Credentials = map[string]any{"api_key": "fixture-account-" + strconv.Itoa(i+1), "base_url": "https://api.example.test"}
			accounts[i].Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
		}
	})
}

func TestOpenAIResponsesCCEmptyStreamActuallyCallsNextAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, firstBody := range map[string]string{
		"empty EOF":  "",
		"done only":  "data: [DONE]\n\n",
		"role only":  "data: {\"id\":\"empty\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
		"SSE error":  "data: {\"error\":{\"code\":\"server_error\",\"message\":\"temporarily unavailable, please retry\"}}\n\n",
		"JSON error": "{\"error\":{\"code\":\"server_error\",\"message\":\"temporarily unavailable, please retry\"}}",
	} {
		t.Run(name, func(t *testing.T) {
			upstream := &emptyCCFailoverUpstream{firstBody: firstBody}
			h := newCCEmptyFailoverTestHandler(t, upstream)
			c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h.Responses(c)

			upstream.mu.Lock()
			ids := append([]int64(nil), upstream.ids...)
			auths := append([]string(nil), upstream.auths...)
			paths := append([]string(nil), upstream.paths...)
			models := append([]string(nil), upstream.models...)
			upstream.mu.Unlock()
			require.Equal(t, []int64{1, 2}, ids, "empty success must cause a real request to another account")
			require.Equal(t, []string{"Bearer fixture-account-1", "Bearer fixture-account-2"}, auths)
			require.Equal(t, []string{"/v1/chat/completions", "/v1/chat/completions"}, paths)
			require.Equal(t, []string{"glm-5.3", "glm-5.3"}, models)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), "healthy fallback")
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.created\n"))
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.completed\n"))
			require.NotContains(t, recorder.Body.String(), "response.failed")
		})
	}
}

func TestOpenAIResponsesCCEmptyStreamExhaustionIsFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &emptyCCFailoverUpstream{allEmpty: true}
	h := newCCEmptyFailoverTestHandler(t, upstream)
	c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Responses(c)
	require.Equal(t, []int64{1, 2}, upstream.ids)
	require.Equal(t, 529, recorder.Code, "retain the handler's existing exhausted-upstream error status")
	require.True(t, gjson.GetBytes(recorder.Body.Bytes(), "error").Exists())
	require.NotContains(t, recorder.Body.String(), "response.completed")
}

func TestOpenAIResponsesCCPartialStreamFailsOnceWithoutReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &emptyCCFailoverUpstream{firstBody: "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n"}
	h := newCCEmptyFailoverTestHandler(t, upstream)
	c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Responses(c)
	require.Equal(t, []int64{1}, upstream.ids, "partial output must not be replayed on a second account")
	require.Contains(t, recorder.Body.String(), `"delta":"partial"`)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
	require.NotContains(t, recorder.Body.String(), "response.completed")
	require.NotContains(t, recorder.Body.String(), "data: [DONE]")
}

func TestOpenAIResponsesCCRefusalDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &emptyCCFailoverUpstream{firstBody: "data: {\"choices\":[{\"index\":0,\"delta\":{\"refusal\":\"I cannot help with that\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"}
	h := newCCEmptyFailoverTestHandler(t, upstream)
	c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Responses(c)
	require.Equal(t, []int64{1}, upstream.ids, "a real refusal is not an empty or retryable response")
	require.Contains(t, recorder.Body.String(), "event: response.refusal.delta")
	require.Contains(t, recorder.Body.String(), "I cannot help with that")
	require.Contains(t, recorder.Body.String(), "event: response.completed")
	require.NotContains(t, recorder.Body.String(), "response.failed")
}

func TestOpenAIResponsesCCPrettyJSONRequestErrorDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &emptyCCFailoverUpstream{firstBody: "{\n  \"error\": {\n    \"code\": \"context_length_exceeded\",\n    \"type\": \"invalid_request_error\",\n    \"message\": \"Your input exceeds the context window\"\n  }\n}\n"}
	h := newCCEmptyFailoverTestHandler(t, upstream)
	c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Responses(c)
	require.Equal(t, []int64{1}, upstream.ids, "JSON formatting must not turn a deterministic request error into failover")
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
	require.Contains(t, recorder.Body.String(), "context_length_exceeded")
	require.NotContains(t, recorder.Body.String(), "response.completed")
}

func TestOpenAIResponsesCCCapacityFailureAfterOutputUsesRetryableClientCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, code := range []string{"server_is_overloaded", "slow_down"} {
		t.Run(code, func(t *testing.T) {
			upstream := &emptyCCFailoverUpstream{firstBody: "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"error\":{\"code\":\"" + code + "\",\"message\":\"Server is overloaded\"}}\n\n"}
			h := newCCEmptyFailoverTestHandler(t, upstream)
			c, recorder := newOpenAIResponsesFailoverTestContext(t, nil)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"glm-5.3","input":"hello","stream":true}`))
			c.Request.Header.Set("Content-Type", "application/json")
			h.Responses(c)
			require.Equal(t, []int64{1}, upstream.ids)
			require.Contains(t, recorder.Body.String(), `"code":"server_error"`)
			require.NotContains(t, recorder.Body.String(), `"code":"`+code+`"`)
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			require.NotContains(t, recorder.Body.String(), "response.completed")
		})
	}
}
