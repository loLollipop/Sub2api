//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertedAnthropicUsageFidelity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, start, delta string
		want               ClaudeUsage
	}{
		{"upstream one hour cache write", `{"input_tokens":11,"output_tokens":4,"cache_creation_input_tokens":600,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":600}}`, `{"output_tokens":8}`, ClaudeUsage{InputTokens: 11, OutputTokens: 8, CacheCreationInputTokens: 600, CacheCreation1hTokens: 600}},
		{"final read clears provisional write", `{"input_tokens":11,"output_tokens":4,"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":600}}`, `{"input_tokens":0,"output_tokens":8,"cache_creation_input_tokens":0,"cache_read_input_tokens":600}`, ClaudeUsage{OutputTokens: 8, CacheReadInputTokens: 600}},
		{"fully cached zero output", `{"input_tokens":11,"output_tokens":4,"cache_creation_input_tokens":600}`, `{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":600}`, ClaudeUsage{CacheReadInputTokens: 600}},
		{"mixed cache durations", `{"input_tokens":2,"cache_creation_input_tokens":600,"cache_read_input_tokens":900,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500}}`, `{"output_tokens":8}`, ClaudeUsage{InputTokens: 2, OutputTokens: 8, CacheCreationInputTokens: 600, CacheReadInputTokens: 900, CacheCreation5mTokens: 100, CacheCreation1hTokens: 500}},
		{"partial cache duration update", `{"input_tokens":2,"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500}}`, `{"output_tokens":8,"cache_creation_input_tokens":500,"cache_creation":{"ephemeral_5m_input_tokens":0}}`, ClaudeUsage{InputTokens: 2, OutputTokens: 8, CacheCreationInputTokens: 500, CacheCreation1hTokens: 500}},
		{"all final counts zero", `{"input_tokens":11,"output_tokens":4,"cache_creation_input_tokens":600,"cache_read_input_tokens":900,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, `{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}`, ClaudeUsage{}},
		{"zero aggregate with partial zero TTL", `{"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0}}`, ClaudeUsage{}},
		{"zero aggregate with contradictory TTL", `{"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, ClaudeUsage{}},
		{"zero aggregate with empty TTL", `{"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, `{"cache_creation_input_tokens":0,"cache_creation":{}}`, ClaudeUsage{}},
		{"zero aggregate with null TTL", `{"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_1h_input_tokens":600}}`, `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, ClaudeUsage{}},
		{"empty TTL retains aggregate", `{"cache_creation_input_tokens":600}`, `{"cache_creation":{}}`, ClaudeUsage{CacheCreationInputTokens: 600}},
		{"null TTL retains aggregate", `{"cache_creation_input_tokens":600}`, `{"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, ClaudeUsage{CacheCreationInputTokens: 600}},
		{"split only partial update", `{"input_tokens":2,"output_tokens":4,"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`, ClaudeUsage{InputTokens: 2, OutputTokens: 4, CacheCreationInputTokens: 500, CacheCreation1hTokens: 500}},
		{"null retains initial counts", `{"input_tokens":2,"output_tokens":4,"cache_creation_input_tokens":600,"cache_read_input_tokens":900,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500}}`, `{"input_tokens":null,"output_tokens":null,"cache_creation_input_tokens":null,"cache_read_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null}}`, ClaudeUsage{InputTokens: 2, OutputTokens: 4, CacheCreationInputTokens: 600, CacheReadInputTokens: 900, CacheCreation5mTokens: 100, CacheCreation1hTokens: 500}},
	} {
		for _, protocol := range []string{"chat", "responses"} {
			for _, stream := range []bool{false, true} {
				name := tc.name + "/" + protocol
				if stream {
					name += "/stream"
				}
				t.Run(name, func(t *testing.T) {
					start := `{"type":"message_start","message":{"id":"msg_cache","type":"message","role":"assistant","content":[],"model":"claude-test","usage":` + tc.start + `}}`
					delta := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + tc.delta + `}`
					sse := "event: message_start\ndata: " + start + "\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"OK\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: " + delta + "\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					var nativeUsage ClaudeUsage
					parseSSEUsagePassthrough(start, &nativeUsage)
					parseSSEUsagePassthrough(delta, &nativeUsage)
					require.Equal(t, tc.want, nativeUsage, "native stream billing is the comparison baseline")
					var generalUsage ClaudeUsage
					newMinimalGatewayService().parseSSEUsage(start, &generalUsage)
					newMinimalGatewayService().parseSSEUsage(delta, &generalUsage)
					require.Equal(t, tc.want, generalUsage, "the general Messages path must use the same final usage")
					upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}}
					cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
					svc := &GatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					path := "/v1/chat/completions"
					payload := map[string]any{"model": "claude-test", "messages": []map[string]string{{"role": "user", "content": "hello"}}, "stream": stream, "stream_options": map[string]bool{"include_usage": true}}
					if protocol == "responses" {
						path = "/v1/responses"
						payload = map[string]any{"model": "claude-test", "input": "hello", "stream": stream}
					}
					c.Request = httptest.NewRequest(http.MethodPost, path, nil)
					c.Set("api_key", &APIKey{Group: &Group{CrossProtocolConversionEnabled: true}})
					body, err := json.Marshal(payload)
					require.NoError(t, err)
					parsed := &ParsedRequest{Body: NewRequestBodyRef(body), Model: "claude-test"}
					var result *ForwardResult
					if protocol == "chat" {
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, newAnthropicAPIKeyAccountForTest(), body, parsed)
					} else {
						result, err = svc.ForwardAsResponses(context.Background(), c, newAnthropicAPIKeyAccountForTest(), body, parsed)
					}
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Equal(t, "/v1/messages", upstream.lastReq.URL.Path)
					require.Equal(t, tc.want, result.Usage)
					billing := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{
						"claude-test": {SupportsCacheBreakdown: true, CacheCreation5mPrice: 4e-6, CacheCreation1hPrice: 5e-6},
					}}
					creationCost := billing.computeCacheCreationCost(billing.fallbackPrices["claude-test"], UsageTokens{
						CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
						CacheCreation5mTokens: result.Usage.CacheCreation5mTokens,
						CacheCreation1hTokens: result.Usage.CacheCreation1hTokens,
					}, 4e-6, 1)
					if tc.want.CacheCreationInputTokens == 0 {
						require.Zero(t, creationCost, "final zero cache creation must incur no TTL write charge")
					} else {
						require.Positive(t, creationCost, "empty TTL updates cannot erase valid cache creation charges")
					}
					wireUsage := convertedAnthropicResponseUsage(rec.Body.String(), stream, protocol)
					require.True(t, wireUsage.Exists(), "converted response must contain final usage")
					creationPath, readPath := "cache_creation_input_tokens", "input_tokens_details.cached_tokens"
					if protocol == "chat" {
						creationPath, readPath = "prompt_tokens_details.cache_creation_tokens", "prompt_tokens_details.cached_tokens"
					}
					require.Equal(t, tc.want.CacheCreationInputTokens, int(wireUsage.Get(creationPath).Int()))
					require.Equal(t, tc.want.CacheReadInputTokens, int(wireUsage.Get(readPath).Int()))
					require.Equal(t, tc.want.CacheCreation1hTokens, int(wireUsage.Get("cache_creation.ephemeral_1h_input_tokens").Int()))
					require.Equal(t, tc.want.CacheCreation5mTokens, int(wireUsage.Get("cache_creation.ephemeral_5m_input_tokens").Int()))
					inputPath, outputPath := "input_tokens", "output_tokens"
					if protocol == "chat" {
						inputPath, outputPath = "prompt_tokens", "completion_tokens"
					}
					totalInput := tc.want.InputTokens + tc.want.CacheReadInputTokens + tc.want.CacheCreationInputTokens
					require.Equal(t, totalInput, int(wireUsage.Get(inputPath).Int()))
					require.Equal(t, tc.want.OutputTokens, int(wireUsage.Get(outputPath).Int()))
					require.Equal(t, totalInput+tc.want.OutputTokens, int(wireUsage.Get("total_tokens").Int()))
					if protocol == "responses" {
						require.Equal(t, tc.want.CacheCreation5mTokens, int(wireUsage.Get("cache_creation_5m_tokens").Int()))
						require.Equal(t, tc.want.CacheCreation1hTokens, int(wireUsage.Get("cache_creation_1h_tokens").Int()))
					}

					// The native OpenAI account handlers share the same converters but
					// have separate buffered assembly and final usage results.
					nativeRec := httptest.NewRecorder()
					nativeContext, _ := gin.CreateTestContext(nativeRec)
					nativeContext.Request = httptest.NewRequest(http.MethodPost, path, nil)
					nativeSvc := &OpenAIGatewayService{cfg: cfg}
					nativeResp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(sse))}
					var nativeResult *OpenAIForwardResult
					switch {
					case protocol == "chat" && stream:
						nativeResult, err = nativeSvc.handleCCStreamingFromNativeAnthropic(nativeResp, nativeContext, "public-model", "billing-model", "claude-test", nil, time.Now(), true)
					case protocol == "chat":
						nativeResult, err = nativeSvc.handleCCBufferedFromNativeAnthropic(nativeResp, nativeContext, "public-model", "billing-model", "claude-test", nil, time.Now())
					case stream:
						nativeResult, err = nativeSvc.handleResponsesStreamingFromNativeAnthropic(nativeResp, nativeContext, "public-model", "billing-model", "claude-test", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					default:
						nativeResult, err = nativeSvc.handleResponsesBufferedFromNativeAnthropic(nativeResp, nativeContext, "public-model", "billing-model", "claude-test", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
					}
					require.NoError(t, err)
					require.Equal(t, claudeUsageToOpenAIUsage(&tc.want), nativeResult.Usage)
					require.Equal(t, "public-model", nativeResult.Model)
					require.Equal(t, "billing-model", nativeResult.BillingModel)
					require.Equal(t, "claude-test", nativeResult.UpstreamModel)
					nativeWireUsage := convertedAnthropicResponseUsage(nativeRec.Body.String(), stream, protocol)
					require.JSONEq(t, wireUsage.Raw, nativeWireUsage.Raw)
				})
			}
		}
	}
}

type convertedAnthropicCountingUpstream struct {
	*anthropicHTTPUpstreamRecorder
	requests int
}

func (u *convertedAnthropicCountingUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.requests++
	return u.anthropicHTTPUpstreamRecorder.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *convertedAnthropicCountingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type convertedAnthropicDisconnectWriter struct {
	gin.ResponseWriter
	writes int
	failed chan struct{}
}

func (w *convertedAnthropicDisconnectWriter) Write(_ []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		close(w.failed)
	}
	return 0, errors.New("client disconnected")
}

func (w *convertedAnthropicDisconnectWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestConvertedAnthropicUsageAfterDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			start := `{"type":"message_start","message":{"id":"msg_disconnect","model":"claude-test","usage":{"input_tokens":2,"output_tokens":8,"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_1h_input_tokens":600}}}}`
			final := `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":19,"cache_creation_input_tokens":0,"cache_read_input_tokens":600}}`
			reader, producer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = producer.Close() })
			failed := make(chan struct{})
			produced := make(chan error, 1)
			go func() {
				defer func() { _ = producer.Close() }()
				if _, err := fmt.Fprintf(producer, "event: message_start\ndata: %s\n\n", start); err != nil {
					produced <- err
					return
				}
				// The final counts are unavailable until the client write fails.
				select {
				case <-failed:
				case <-time.After(3 * time.Second):
					produced <- errors.New("client did not disconnect")
					return
				}
				_, err := fmt.Fprintf(producer, "event: message_delta\ndata: %s\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", final)
				produced <- err
			}()
			upstream := &convertedAnthropicCountingUpstream{anthropicHTTPUpstreamRecorder: &anthropicHTTPUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: reader}}}
			svc := newMinimalGatewayService()
			svc.httpUpstream = upstream
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+protocol, nil)
			c.Set("api_key", &APIKey{Group: &Group{CrossProtocolConversionEnabled: true}})
			writer := &convertedAnthropicDisconnectWriter{ResponseWriter: c.Writer, failed: failed}
			c.Writer = writer
			payload := `{"model":"claude-test","stream":true,"messages":[{"role":"user","content":"hello"}],"stream_options":{"include_usage":true}}`
			if protocol == "responses" {
				payload = `{"model":"claude-test","stream":true,"input":"hello"}`
			}
			var result *ForwardResult
			var err error
			if protocol == "chat" {
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(payload), nil)
			} else {
				result, err = svc.ForwardAsResponses(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(payload), nil)
			}
			require.NoError(t, err)
			require.NoError(t, <-produced)
			require.True(t, result.ClientDisconnect)
			require.Equal(t, ClaudeUsage{OutputTokens: 19, CacheReadInputTokens: 600}, result.Usage)
			require.Equal(t, 1, upstream.requests, "continue draining the original upstream without replay")
			require.Equal(t, 1, writer.writes, "no client writes after the first failure")
		})
	}
}

func convertedAnthropicResponseUsage(body string, stream bool, protocol string) gjson.Result {
	if !stream {
		return gjson.Get(body, "usage")
	}
	var usage gjson.Result
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		event := gjson.Parse(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		path := "usage"
		if protocol == "responses" {
			path = "response.usage"
		}
		if current := event.Get(path); current.Exists() {
			usage = current
		}
	}
	return usage
}

func TestConvertedAnthropicUsageParserPresence(t *testing.T) {
	for _, parser := range []struct {
		name  string
		parse func(string, *ClaudeUsage)
	}{
		{"passthrough", parseSSEUsagePassthrough},
		{"messages", newMinimalGatewayService().parseSSEUsage},
	} {
		t.Run(parser.name, func(t *testing.T) {
			var usage ClaudeUsage
			parser.parse(`{"type":"message_start","message":{"usage":{"input_tokens":11,"output_tokens":4,"cache_creation_input_tokens":600,"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500}}}}`, &usage)
			require.Equal(t, 4, usage.OutputTokens, "message_start output must be retained")
			parser.parse(`{"type":"message_start","message":{"usage":{"input_tokens":null,"cache_read_input_tokens":null}}}`, &usage)
			require.Equal(t, 11, usage.InputTokens, "omitted and null fields retain preceding usage")
			require.Equal(t, 600, usage.CacheCreationInputTokens)
			parser.parse(`{"type":"message_delta","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":600}}`, &usage)
			require.Equal(t, ClaudeUsage{CacheReadInputTokens: 600}, usage, "explicit zero replaces provisional counts and clears old TTL buckets")
		})
	}
}

func TestConvertedAnthropicUsageParserAliases(t *testing.T) {
	for _, tc := range []struct {
		name, delta string
		want        ClaudeUsage
	}{
		{"Kimi fully cached", `{"input_tokens":0,"output_tokens":8,"cache_read_input_tokens":600,"prompt_tokens":600,"cached_tokens":600}`, ClaudeUsage{OutputTokens: 8, CacheReadInputTokens: 600}},
		{"Kimi canonical zero and alias", `{"input_tokens":2,"cache_read_input_tokens":0,"cached_tokens":128}`, ClaudeUsage{InputTokens: 2, OutputTokens: 4, CacheReadInputTokens: 128}},
		{"GLM", `{"input_tokens":1200,"output_tokens":30,"prompt_tokens":1200,"prompt_tokens_details":{"cached_tokens":800}}`, ClaudeUsage{InputTokens: 400, OutputTokens: 30, CacheReadInputTokens: 800}},
		{"DeepSeek", `{"input_tokens":1200,"output_tokens":30,"prompt_cache_hit_tokens":800,"prompt_cache_miss_tokens":400}`, ClaudeUsage{InputTokens: 400, OutputTokens: 30, CacheReadInputTokens: 800}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := `{"type":"message_start","message":{"usage":{"input_tokens":600,"output_tokens":4}}}`
			delta := `{"type":"message_delta","usage":` + tc.delta + `}`
			var passthrough, messages, converted ClaudeUsage
			parseSSEUsagePassthrough(start, &passthrough)
			parseSSEUsagePassthrough(delta, &passthrough)
			svc := newMinimalGatewayService()
			svc.parseSSEUsage(start, &messages)
			svc.parseSSEUsage(delta, &messages)
			var event apicompat.AnthropicStreamEvent
			require.NoError(t, json.Unmarshal([]byte(start), &event))
			mergeAnthropicUsage(&converted, event.Message.Usage)
			require.NoError(t, json.Unmarshal([]byte(delta), &event))
			mergeAnthropicUsage(&converted, *event.Usage)
			require.Equal(t, tc.want, passthrough)
			require.Equal(t, tc.want, messages)
			require.Equal(t, tc.want, converted)
		})
	}
}
