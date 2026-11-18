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
)

func TestRelayPrivacyLocalStreamBoundariesKeepUsageAndEvidence(t *testing.T) {
	const message = "bad input (request id: Synthetic123)"
	const failure = `{"type":"error","error":{"type":"new_api_error","message":"bad input (request id: Synthetic123)"}}`
	for _, protocol := range []string{"antigravity", "native", "images"} {
		for _, named := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "/data", true: "/event"}[named], func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				header := ""
				if named {
					header = "event: error\n"
				}
				prefix := "data: " + `{"type":"response.output_text.delta","delta":"new_api_error (request id: Synthetic123)"}` + "\n\n"
				switch protocol {
				case "antigravity":
					prefix = "data: " + `{"response":{"candidates":[{"content":{"parts":[{"text":"new_api_error (request id: Synthetic123)"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":3}}}` + "\n\n"
				case "native":
					prefix = "event: message_start\ndata: " + `{"type":"message_start","message":{"usage":{"input_tokens":11,"cache_read_input_tokens":7}}}` + "\n\nevent: content_block_delta\ndata: " + `{"type":"content_block_delta","delta":{"type":"text_delta","text":"new_api_error (request id: Synthetic123)"}}` + "\n\n"
				case "images":
					prefix = "data: " + `{"type":"image_generation.partial_image","partial_image_b64":"new_api_error (request id: Synthetic123)","usage":{"input_tokens":11,"output_tokens":3}}` + "\n\n"
				}
				resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(prefix + header + "data: " + failure + "\n\n"))}
				cfg := &config.Config{}
				cfg.Gateway.LogUpstreamErrorBody = true
				svc := &OpenAIGatewayService{cfg: cfg}
				switch protocol {
				case "antigravity":
					ag := &AntigravityGatewayService{settingService: &SettingService{cfg: cfg}}
					result, err := ag.handleGeminiStreamingResponse(c, resp, time.Now())
					require.NoError(t, err)
					require.Equal(t, 11, result.usage.InputTokens)
					require.Equal(t, 3, result.usage.OutputTokens)
				case "native":
					result, err := svc.handleNativeAnthropicStreamingResponse(t.Context(), resp, c, &Account{ID: 702, Platform: PlatformKimi, Type: AccountTypeAPIKey, Concurrency: 1}, "k3", "k3", "k3", nil, time.Now())
					require.NoError(t, err)
					require.Equal(t, 18, result.Usage.InputTokens)
					require.Equal(t, 7, result.Usage.CacheReadInputTokens)
				case "images":
					usage, _, _, _, err := svc.handleOpenAIImagesStreamingResponse(resp, c, time.Now())
					require.NoError(t, err)
					require.Equal(t, 11, usage.InputTokens)
					require.Equal(t, 3, usage.OutputTokens)
				}
				output := rec.Body.String()
				require.Contains(t, output, "new_api_error (request id: Synthetic123)", "successful content must remain untouched")
				require.Contains(t, output, `"message":"bad input"`)
				require.NotContains(t, output, `"type":"new_api_error"`)
				if named {
					require.Contains(t, output, "event: error\n")
				}
				if protocol != "images" {
					require.Equal(t, message, c.GetString(OpsUpstreamErrorMessageKey))
					require.Equal(t, failure, c.GetString(OpsUpstreamErrorDetailKey))
					marked, ok := GetOpsStreamError(c)
					require.True(t, ok)
					require.Equal(t, message, marked.Message)
				}
			})
		}
	}
}

func TestRelayPrivacyImagesEventWriterKeepsSuccessfulPayload(t *testing.T) {
	const payload = `{"type":"new_api_error","message":"bad input (request id: Synthetic123)"}`
	for _, event := range []string{"error", "image_generation.completed"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		svc := &OpenAIGatewayService{}
		require.NoError(t, svc.writeOpenAIImagesStreamEvent(c, rec, event, []byte(payload)))
		if event == "error" {
			require.NotContains(t, rec.Body.String(), "new_api_error")
			require.NotContains(t, rec.Body.String(), "request id:")
		} else {
			require.Contains(t, rec.Body.String(), payload)
		}
	}
}

func TestRelayPrivacyIngressRealWebSocketKeepsRawTerminalAndUsage(t *testing.T) {
	const success = `{"type":"response.output_text.delta","response_id":"resp_privacy","delta":"new_api_error (request id: Synthetic123)"}`
	const failure = `{"type":"response.failed","response":{"id":"resp_privacy","model":"gpt-5.1","status":"failed","error":{"type":"new_api_error","message":"bad input (request id: Synthetic123)"},"usage":{"input_tokens":13,"output_tokens":3}}}`
	upstream := &openAIWSCaptureConn{events: [][]byte{[]byte(success), []byte(failure)}}
	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.LogUpstreamErrorBody = true
	dialer := &openAIWSCaptureDialer{conn: upstream}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
	account := &Account{ID: 5885, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-token"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
	type turnEvidence struct {
		result  *OpenAIForwardResult
		err     error
		message string
	}
	turns := make(chan turnEvidence, 1)
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, first, err := conn.Read(ctx)
		if err != nil {
			done <- err
			return
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r.Clone(ctx)
		hooks := &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
			marked, _ := GetOpsStreamError(c)
			turns <- turnEvidence{result: result, err: err, message: marked.Message}
		}}
		done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "fixture-token", first, hooks)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","input":[]}`)))
	_, message, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, success, string(message))
	_, message, err = conn.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(message), `"message":"bad input"`)
	require.NotContains(t, string(message), "new_api_error")
	require.NotContains(t, string(message), "request id:")
	select {
	case turn := <-turns:
		require.NoError(t, turn.err)
		require.NotNil(t, turn.result)
		require.Equal(t, 13, turn.result.Usage.InputTokens)
		require.Equal(t, 3, turn.result.Usage.OutputTokens)
		require.Equal(t, "response.failed", turn.result.UpstreamTerminalEvent)
		require.Contains(t, turn.message, "request id: Synthetic123")
	case <-ctx.Done():
		t.Fatal("websocket turn did not finish")
	}
	_ = conn.CloseNow()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("websocket relay did not stop")
	}
	require.Equal(t, 1, dialer.DialCount(), "error privacy must not replay a billed turn")
}

func TestRelayPrivacyToolAndEmbeddingErrorsKeepOriginalOps(t *testing.T) {
	const failure = `{"error":{"type":"new_api_error","message":"bad input (request id: Synthetic123)"}}`
	const success = `{"results":[{"type":"new_api_error","message":"sample (request id: Synthetic123)"}]}`
	for _, protocol := range []string{"tool", "embedding"} {
		for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
			t.Run(protocol+"/"+http.StatusText(status), func(t *testing.T) {
				body := failure
				if status == http.StatusOK {
					body = success
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, DeepSeekToolsPathSearch, nil)
				resp := &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				if protocol == "embedding" {
					writeOpenAIEmbeddingsUpstreamResponse(c, resp, []byte(body), nil)
				} else {
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: resp}}
					result, err := svc.ForwardDeepSeekTools(t.Context(), c, deepSeekToolsTestAccount(), DeepSeekToolsPathSearch, []byte(`{"query":"q"}`))
					require.NoError(t, err)
					if status >= 400 {
						require.Nil(t, result)
					} else {
						require.NotNil(t, result)
					}
				}
				if status >= 400 {
					require.NotContains(t, rec.Body.String(), "new_api_error")
					require.NotContains(t, rec.Body.String(), "request id:")
					require.Equal(t, "bad input (request id: Synthetic123)", c.GetString(OpsUpstreamErrorMessageKey))
					require.Equal(t, body, c.GetString(OpsUpstreamErrorDetailKey))
				} else {
					require.Equal(t, body, rec.Body.String())
					require.Empty(t, c.GetString(OpsUpstreamErrorMessageKey))
					require.Empty(t, c.GetString(OpsUpstreamErrorDetailKey))
				}
			})
		}
	}
}
