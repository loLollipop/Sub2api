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
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSelectedOpusMessagesOriginalFactsAndMapping(t *testing.T) {
	for _, tc := range []struct {
		name, raw, repaired, selected string
		invalid                       bool
	}{
		{"missing max survives defaults", `{"messages":[{"role":"user","content":"hi"}]}`, `{"max_tokens":128000,"messages":[{"role":"user","content":"hi"}]}`, "claude-opus-5-5", true},
		{"empty text survives stripping", `{"max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":" "},{"type":"text","text":"hi"}]}]}`, `{"max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`, "claude-opus-5-5", true},
		{"mapped away not falsely rejected", `{"messages":[]}`, `{"messages":[]}`, "deepseek-v4", false},
		{"legacy enabled stays compatible", `{"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":50},"messages":[{"role":"user","content":"hi"}]}`, "", "claude-opus-5-5", false},
		{"numeric text is invalid", `{"max_tokens":100,"messages":[{"role":"user","content":[{"type":"text","text":12}]}]}`, "", "claude-opus-5-5", true},
		{"fractional max is invalid", `{"max_tokens":1.5,"messages":[{"role":"user","content":"hi"}]}`, "", "claude-opus-5-5", true},
		{"implicit thinking sampling", `{"max_tokens":100,"temperature":0.5,"messages":[{"role":"user","content":"hi"}]}`, "", "claude-opus-5-5", true},
		{"haiku enabled is not a legacy normalization", `{"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":50},"messages":[{"role":"user","content":"hi"}]}`, "", "claude-haiku-5-5", true},
		{"sonnet enabled compatibility preserved", `{"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":50},"messages":[{"role":"user","content":"hi"}]}`, "", "claude-sonnet-5-5", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			CaptureMessagesValidationFacts(c, []byte(tc.raw))
			body := tc.raw
			if tc.repaired != "" {
				body = tc.repaired
			}
			err := validateSelectedAnthropicMessagesRequest(c, []byte(body), tc.selected)
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	require.NoError(t, validateSelectedAnthropicMessagesRequest(nil, []byte(`{}`), "deepseek-v4"))
}

func TestMappedOpusValidationBeforeUpstreamOnBothMessagesPaths(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, pass := range []bool{false, true} {
			name := "anthropic"
			if native {
				name = "openai-native"
			}
			if pass {
				name += "-passthrough"
			}
			t.Run(name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := []byte(`{"model":"public-alias","messages":[{"role":"user","content":"hi"}]}`)
				CaptureMessagesValidationFacts(c, body)
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{
					"api_key": "not-a-real-key", "base_url": "https://api.anthropic.com", "model_mapping": map[string]any{"public-alias": "claude-opus-5-5"},
				}, Extra: map[string]any{"anthropic_passthrough": pass}}
				var err error
				if native {
					account.Platform = PlatformMixed
					_, err = (&OpenAIGatewayService{}).forwardAnthropicViaNativeAnthropicEndpoint(context.Background(), c, account, body, "")
				} else {
					_, err = (&GatewayService{}).Forward(context.Background(), c, account, &ParsedRequest{Model: "public-alias", Body: NewRequestBodyRef(body)})
				}
				require.ErrorContains(t, err, "max_tokens")
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), "invalid_request_error")
				// Services have no transport configured: any upstream call would
				// panic rather than satisfy the validation assertions above.
			})
		}
	}
}

func TestClaude55BufferedResponsePreservesSignatureAtServiceBoundary(t *testing.T) {
	const sse = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"model\":\"alias\",\"content\":[],\"usage\":{\"input_tokens\":1}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"signed\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "anthropic", true: "openai-native"}[native], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
			cfg := &config.Config{}
			if native {
				svc := &OpenAIGatewayService{cfg: cfg}
				_, err := svc.handleResponsesBufferedFromNativeAnthropic(resp, c, "alias", "claude-haiku-5-5", "claude-haiku-5-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{}, true)
				require.NoError(t, err)
			} else {
				svc := &GatewayService{cfg: cfg}
				_, err := svc.handleResponsesBufferedStreamingResponse(resp, c, "alias", "claude-haiku-5-5", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				require.NoError(t, err)
			}
			require.Contains(t, rec.Body.String(), "anthropic-thinking-v1:")
			require.Contains(t, rec.Body.String(), `"model":"alias"`)
		})
	}
}
