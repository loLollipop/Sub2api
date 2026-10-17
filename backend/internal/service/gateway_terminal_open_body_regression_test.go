//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type terminalOpenBodyResult struct {
	input, output, cached int
	disconnected          bool
	err                   error
}

func startTerminalOpenBodyStream(t *testing.T, platform string, passthrough bool, recorder *openAIResponseFlushRecorder) (*io.PipeWriter, <-chan terminalOpenBodyResult) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: reader}
	account := &Account{ID: 1, Platform: platform, Type: AccountTypeAPIKey}
	results := make(chan terminalOpenBodyResult, 1)
	go func() {
		// Forward closes the response body on return, including main Anthropic.
		defer reader.Close()
		out := terminalOpenBodyResult{}
		if platform == PlatformAnthropic {
			svc := newStreamingResponseTestGatewayService()
			var result *streamingResult
			if passthrough {
				result, out.err = svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, account, time.Now(), "model")
			} else {
				result, out.err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model", false)
			}
			if result != nil {
				out.input, out.output, out.cached = result.usage.InputTokens, result.usage.OutputTokens, result.usage.CacheReadInputTokens
				out.disconnected = result.clientDisconnect
			}
		} else {
			svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
			if passthrough {
				result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "model", "model")
				out.err = err
				if result != nil {
					out.input, out.output, out.cached = result.usage.InputTokens, result.usage.OutputTokens, result.usage.CacheReadInputTokens
					out.disconnected = result.clientDisconnect
				}
			} else {
				result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model")
				out.err = err
				if result != nil {
					out.input, out.output, out.cached = result.usage.InputTokens, result.usage.OutputTokens, result.usage.CacheReadInputTokens
					out.disconnected = result.clientDisconnect
				}
			}
		}
		results <- out
	}()
	return writer, results
}

func requireTerminalOpenBodyResult(t *testing.T, results <-chan terminalOpenBodyResult) terminalOpenBodyResult {
	t.Helper()
	synctest.Wait()
	select {
	case result := <-results:
		return result
	default:
		t.Fatal("terminal frame must finish the request while the upstream body remains open")
		return terminalOpenBodyResult{}
	}
}

func TestTerminalOpenBodyCompletesOnlyAfterFullFrame(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		terminals := []string{"message_stop", "[DONE]"}
		if platform == PlatformOpenAI {
			terminals = []string{"response.completed", "response.done", "[DONE]"}
		}
		for _, passthrough := range []bool{false, true} {
			for _, terminal := range terminals {
				t.Run(fmt.Sprintf("%s/passthrough_%t/%s", platform, passthrough, terminal), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						recorder := newOpenAIResponseFlushRecorder()
						writer, results := startTerminalOpenBodyStream(t, platform, passthrough, recorder)
						preamble := "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12,\"cache_read_input_tokens\":3}}}\n\n" +
							"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":4}}\n\n"
						terminalFrame := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n"
						if platform == PlatformOpenAI {
							preamble = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
							terminalFrame = "event: " + terminal + "\ndata: " + strings.Replace(timingReviewTerminal, "response.completed", terminal, 1) + "\n"
							if terminal == "[DONE]" {
								preamble += "data: {\"usage\":{\"input_tokens\":12,\"output_tokens\":4,\"input_tokens_details\":{\"cached_tokens\":3}}}\n\n"
							}
						}
						if terminal == "[DONE]" {
							terminalFrame = "data: [DONE]\n"
						}
						_, err := io.WriteString(writer, preamble+terminalFrame)
						require.NoError(t, err)
						synctest.Wait()
						select {
						case result := <-results:
							t.Fatalf("returned before terminal frame boundary: %+v", result)
						default:
						}
						_, err = io.WriteString(writer, "\n")
						require.NoError(t, err)
						result := requireTerminalOpenBodyResult(t, results)
						require.NoError(t, result.err)
						require.False(t, result.disconnected)
						require.Equal(t, 12, result.input)
						require.Equal(t, 4, result.output)
						require.Equal(t, 3, result.cached)
						_, flushes := recorder.snapshot()
						require.NotEmpty(t, flushes)
						require.Contains(t, flushes[len(flushes)-1], terminalFrame+"\n", "the entire terminal frame must reach the client before returning")
					})
				})
			}
		}
	}
}

func TestTerminalOpenBodyBareErrorWaitsForAuthoritativeCompletion(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough_%t", passthrough), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				recorder := newOpenAIResponseFlushRecorder()
				writer, results := startTerminalOpenBodyStream(t, PlatformOpenAI, passthrough, recorder)
				_, err := io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"+
					"event: error\ndata: {\"type\":\"error\",\"message\":\"temporary warning\"}\n\n")
				require.NoError(t, err)
				synctest.Wait()
				select {
				case result := <-results:
					t.Fatalf("bare error must wait for authoritative completion: %+v", result)
				default:
				}
				_, err = io.WriteString(writer, "data: "+timingReviewTerminal+"\n\n")
				require.NoError(t, err)
				result := requireTerminalOpenBodyResult(t, results)
				require.NoError(t, result.err)
				require.Equal(t, 12, result.input)
				require.Equal(t, 4, result.output)
				body, _ := recorder.snapshot()
				require.NotContains(t, body, "temporary warning")
				require.NotContains(t, body, "response.failed")
			})
		})
	}
}

func TestTerminalOpenBodyFailureAndDisconnectedUsage(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, disconnected := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough_%t/disconnected_%t", passthrough, disconnected), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					recorder := newOpenAIResponseFlushRecorder()
					if disconnected {
						recorder.failAfterWrites = 0
					}
					writer, results := startTerminalOpenBodyStream(t, PlatformOpenAI, passthrough, recorder)
					_, err := io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
					require.NoError(t, err)
					synctest.Wait()
					terminal := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"created_at\":1700000000,\"usage\":{\"input_tokens\":9,\"output_tokens\":2},\"error\":{\"code\":\"cyber_policy\",\"message\":\"blocked by policy\"}}}\n\n"
					_, err = io.WriteString(writer, terminal)
					require.NoError(t, err)
					result := requireTerminalOpenBodyResult(t, results)
					require.ErrorContains(t, result.err, "upstream response failed")
					require.Equal(t, 9, result.input)
					require.Equal(t, 2, result.output)
					require.Equal(t, disconnected, result.disconnected)
					if !disconnected {
						_, flushes := recorder.snapshot()
						require.NotEmpty(t, flushes)
						// Failure usage remains billable internally, but is stripped
						// from the client frame by the existing response sanitizer.
						wantTerminal := strings.Replace(terminal, `"usage":{"input_tokens":9,"output_tokens":2},`, "", 1)
						require.Contains(t, flushes[len(flushes)-1], wantTerminal)
					}
				})
			})
		}
	}
}
