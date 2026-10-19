package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesWebSearchAlias(t *testing.T) {
	t.Run("preserves all other bytes and does not mutate input", func(t *testing.T) {
		body := []byte(`{ "tools": [{"type":"function","name":"web_search_preview","parameters":{"type":"object"}}, {"type":"web_search_preview_2025_03_11","search_context_size":"low","user_location":{"type":"approximate","country":"US"}}], "tool_choice":{"type":"web_search_preview"}, "input":"web_search_preview", "nonce":9007199254740993 }`)
		original := string(body)
		out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType(body)
		require.NoError(t, err)
		require.True(t, changed)
		want := strings.Replace(original, `"type":"web_search_preview_2025_03_11"`, `"type":"web_search"`, 1)
		want = strings.Replace(want, `"type":"web_search_preview"`, `"type":"web_search"`, 1)
		require.Equal(t, want, string(out))
		require.Equal(t, original, string(body))
	})
	t.Run("all declarations and escaped names", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"web_search_previ\u0065w"},{"type":"web_search_preview"}],"tool_choice":{"type":"web_search_preview_2025_03_11"}}`)
		out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType(body)
		require.NoError(t, err)
		require.True(t, changed)
		for _, path := range []string{"tools.0.type", "tools.1.type", "tool_choice.type"} {
			require.Equal(t, "web_search", gjson.GetBytes(out, path).String())
		}
	})
	t.Run("escaped field names", func(t *testing.T) {
		out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType([]byte(`{"\u0074ools":[{"\u0074ype":"web_search_previ\u0065w"}],"tool_\u0063hoice":{"type":"web_search_previ\u0065w"}}`))
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "web_search", gjson.GetBytes(out, "tools.0.type").String())
		require.Equal(t, "web_search", gjson.GetBytes(out, "tool_choice.type").String())
	})
	for _, body := range []string{
		``, ` {"tools":[{"type":"web_search"}]} `,
		`{"input":"web_search_preview","tools":[{"type":"function","name":"web_search_preview"}],"tool_choice":"auto"}`,
		`{"tools":[{"type":"web_search_preview_custom"},{"type":"web_search_previews"},{"type":"WEB_SEARCH_PREVIEW"},{"type":" web_search_preview "},{"type":"web_search_preview_2025_13_01"},{"type":null}],"tool_choice":{"type":"function","name":"web_search_preview"}}`,
		`{"tools":{"type":"web_search_preview"},"tool_choice":"web_search_preview","input":[{"type":"web_search_call","action":{"type":"search"}}]}`,
	} {
		t.Run("no-op/"+body, func(t *testing.T) {
			input := []byte(body)
			out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType(input)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, string(out))
			if len(input) > 0 {
				require.Same(t, &input[0], &out[0])
			}
		})
	}
	t.Run("invalid matching JSON is never repaired", func(t *testing.T) {
		body := []byte(`{"tools":[{"type":"web_search_preview"}],`)
		out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType(body)
		require.Error(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	})
	t.Run("nil", func(t *testing.T) {
		out, changed, err := normalizeOpenAIResponsesLegacyWebSearchToolType(nil)
		require.NoError(t, err)
		require.False(t, changed)
		require.Nil(t, out)
	})
}

func TestOpenAIResponsesWebSearchAliasHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "passthrough"}[passthrough], func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"id":"resp_search","status":"completed","output":[{"type":"web_search_call","id":"ws_1","status":"completed"}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
			}}
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "test-key", "base_url": "https://example.com"},
				Extra:       map[string]any{"use_responses_api": true, "openai_passthrough": passthrough}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			body := []byte(`{"model":"gpt-5.1","stream":false,"input":"search","tools":[{"type":"web_search_preview_2025_03_11","search_context_size":"low"}],"tool_choice":{"type":"web_search_preview"}}`)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
			require.Equal(t, "low", gjson.GetBytes(upstream.lastBody, "tools.0.search_context_size").String())
			require.Equal(t, "web_search_call", gjson.GetBytes(rec.Body.Bytes(), "output.0.type").String())
		})
	}
}

func TestOpenAIResponsesWebSearchAliasWSCompatibility(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.1","input":[],"tools":[{"type":"web_search_preview"}],"tool_choice":{"type":"web_search_preview_2025_03_11"}}`)
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth, AccountTypeSetupToken} {
		out, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: PlatformOpenAI, Type: accountType}, false)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "web_search", gjson.GetBytes(out, "tools.0.type").String())
		require.Equal(t, "web_search", gjson.GetBytes(out, "tool_choice.type").String())
	}
	for _, platform := range []string{PlatformDeepseek, PlatformGrok, PlatformGemini, PlatformAnthropic} {
		out, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{Platform: platform, Type: AccountTypeAPIKey}, false)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}
}

func TestOpenAIResponsesWebSearchAliasWSPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_alias","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	server, serverErr := startPassthroughLifecycleServer(t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream), passthroughLifecycleAccount())
	defer server.Close()
	clientConn := dialPassthroughLifecycleClientWithPayload(t, server, `{"type":"response.create","model":"gpt-5.1","input":[],"tools":[{"type":"web_search_preview"}],"tool_choice":{"type":"web_search_preview_2025_03_11"}}`)
	defer func() { _ = clientConn.CloseNow() }()
	upstreamBody := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "web_search", gjson.GetBytes(upstreamBody, "tools.0.type").String())
	require.Equal(t, "web_search", gjson.GetBytes(upstreamBody, "tool_choice.type").String())
	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_alias","tools":[{"type":"web_search_preview_2025_03_11"}],"tool_choice":{"type":"web_search_preview"}}`))
	cancelWrite()
	require.NoError(t, err)
	secondBody := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "web_search", gjson.GetBytes(secondBody, "tools.0.type").String())
	require.Equal(t, "web_search", gjson.GetBytes(secondBody, "tool_choice.type").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_alias_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	event, err = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_alias_2", gjson.GetBytes(event, "response.id").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("web search alias passthrough did not finish")
	}
}
