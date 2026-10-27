package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayStabilityOverloadedWrapping(t *testing.T) {
	for _, upstreamStatus := range []int{500, 502, 503, 504, 529} {
		status, kind, _, _ := WrapUpstreamErrorForClient(upstreamStatus, []byte(`{"error":{"type":"overloaded_error","message":"Overloaded"}}`))
		want := 529
		if upstreamStatus == 503 {
			want = 503
		}
		require.Equal(t, want, status)
		require.Equal(t, "overloaded_error", kind)
	}
	status, kind, _, message := WrapUpstreamErrorForClient(500, []byte(`{"error":{"type":"overloaded_error","message":"insufficient balance"}}`))
	require.Equal(t, 502, status)
	require.Equal(t, "upstream_error", kind)
	require.Equal(t, UpstreamUnavailableMessage, message)
}

func TestGatewayStabilityAnthropicErrorIsTerminal(t *testing.T) {
	for _, test := range []struct {
		kind string
		want int
	}{
		{"overloaded_error", 529}, {"rate_limit_error", 429}, {"api_error", 500},
	} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/raw=%t", test.kind, raw), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				payload := fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":"try again"}}`, test.kind)
				line := "event: error\ndata: " + payload
				if raw {
					line = payload
				}
				resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(line + "\n\n"))}
				svc := &GatewayService{cfg: &config.Config{}}
				result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-sonnet-4-5")
				var terminalErr *AnthropicPassthroughStreamError
				require.ErrorAs(t, err, &terminalErr)
				require.True(t, terminalErr.ResponseWritten)
				require.Equal(t, test.want, terminalErr.StatusCode)
				require.Equal(t, test.kind, terminalErr.ErrorType)
				var replay *UpstreamFailoverError
				require.NotErrorAs(t, err, &replay)
				require.NotNil(t, result)
				streamErr, ok := GetOpsStreamError(c)
				require.True(t, ok)
				require.Equal(t, test.want, streamErr.IntendedStatus)
				require.Equal(t, test.kind, streamErr.ErrType)
				require.Equal(t, "try again", streamErr.Message)
				require.Contains(t, recorder.Body.String(), payload)
				require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"error"`))
				require.NotContains(t, recorder.Body.String(), "Upstream request failed")
			})
		}
	}
	_, valid := anthropicStreamErrorPayload(`data: {"type":"error","error":`)
	require.False(t, valid, "truncated JSON is not proof of terminal completion")
}

func TestGatewayStabilityAnthropicRawFinancialErrorIsMasked(t *testing.T) {
	for _, line := range []string{`{"type":"error","error":{"type":"api_error","message":"insufficient balance"}}`, `data: {"type":"error","error":{"type":"api_error","message":"insufficient balance"}}`} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(line + "\n\n"))}
		result, err := (&GatewayService{cfg: &config.Config{}}).handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-sonnet-4-5")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotContains(t, strings.ToLower(recorder.Body.String()), "balance")
		require.Contains(t, recorder.Body.String(), UpstreamUnavailableMessage)
	}
}

func TestGatewayStabilityDeepSeekDedupeScopeUnchanged(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"assistant","tool_calls":[{"id":"a"}]},{"role":"tool","tool_call_id":"a","content":"one"},{"role":"assistant","tool_calls":[{"id":"a"}]},{"role":"tool","tool_call_id":"a","content":"two"}]}`)
	require.Equal(t, body, RewriteInboundDeepSeekCallIDs(PlatformOpenAI, "gpt-5.4", body))
	for _, platform := range []string{PlatformDeepseek, PlatformOpenAI} {
		model := "gpt-5.4"
		if platform == PlatformOpenAI {
			model = "deepseek-v3.2"
		}
		out := RewriteInboundDeepSeekCallIDs(platform, model, body)
		require.Len(t, gjson.GetBytes(out, "messages").Array(), 2)
		require.NotContains(t, string(out), "two")
	}
}

func TestGatewayStabilityCompleteJSONArgumentsRemainUntouched(t *testing.T) {
	for _, value := range []string{`{"values":[1,1]}`, `[1,1]`, `{"nested":{"a":[1,2],"b":[1,2]}}`, " \n{\"text\":\"{}{}\"}\n "} {
		require.True(t, json.Valid([]byte(value)))
		_, changed := dedupeRepeatedJSONArgumentString(value)
		require.False(t, changed)
	}
	value := `{"command":"pwd"}`
	out, changed := dedupeRepeatedJSONArgumentString(value + value)
	require.True(t, changed)
	require.Equal(t, value, out)
}

func TestGatewayStabilityImageBridgePlatformGuard(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{CodexImageGenerationBridgeEnabled: true}}}
	for _, platform := range []string{PlatformOpenAI, PlatformKimi, PlatformDeepseek, PlatformMiniMax, PlatformZhipu, PlatformAnthropic} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{Platform: platform, Extra: map[string]any{featureKeyCodexImageGenerationBridge: true}}
			require.Equal(t, platform == PlatformOpenAI, svc.isCodexImageGenerationBridgeEnabled(context.Background(), account, nil))
		})
	}
	require.False(t, svc.isCodexImageGenerationBridgeEnabled(context.Background(), nil, nil))
	account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{featureKeyCodexImageGenerationBridge: false}}
	require.False(t, svc.isCodexImageGenerationBridgeEnabled(context.Background(), account, nil))
}
