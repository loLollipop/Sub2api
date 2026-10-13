//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPassthroughLifecycleHeaderCancelAndBufferedBodies(t *testing.T) {
	for _, phase := range []string{"headers", "error_body", "nonstream_body", "rejected_field"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","input":"hello","stream":%t}`, phase != "nonstream_body"))
				c, _ := newOpenAIPartialUsageContext(t, body)
				c.Request = c.Request.WithContext(ctx)
				resp, tracked, pw := newLifecycleTrackingResponse()
				defer pw.Close()
				if phase == "error_body" || phase == "rejected_field" {
					resp.StatusCode = http.StatusBadRequest
				}
				calls := 0
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}},
					httpUpstream: headerCancelUpstreamFunc(func(req *http.Request) (*http.Response, error) {
						calls++
						if phase == "headers" {
							<-req.Context().Done()
							return nil, req.Context().Err()
						}
						return resp, nil
					}),
				}
				done := make(chan headerCancelForwardResult, 1)
				go func() {
					result, err := svc.Forward(context.WithoutCancel(ctx), c, newOpenAIPartialUsageAccount(true), body)
					done <- headerCancelForwardResult{result, err}
				}()
				synctest.Wait()
				require.Equal(t, 1, calls)
				cancel()
				if phase == "rejected_field" {
					_, err := io.WriteString(pw, `{"error":{"message":"Unsupported parameter: 'temperature'.","param":"temperature"}}`)
					require.NoError(t, err)
					require.NoError(t, pw.Close())
				} else {
					time.Sleep(1100 * time.Millisecond)
				}
				synctest.Wait()
				select {
				case got := <-done:
					require.Error(t, got.err)
					var failover *UpstreamFailoverError
					require.False(t, errors.As(got.err, &failover))
					require.Equal(t, 1, calls, "canceled request must never retry")
				case <-time.After(time.Second):
					t.Fatal("passthrough remained blocked after cancellation budget")
				}
				if phase != "headers" {
					select {
					case <-tracked.closed:
					default:
						t.Fatal("body leaked")
					}
				}
			})
		})
	}
}

func TestPassthroughLifecycleAlreadyCanceledDoesNotSendUpstream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	c, _ := newOpenAIPartialUsageContext(t, body)
	c.Request = c.Request.WithContext(ctx)
	calls := 0
	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
		httpUpstream: headerCancelUpstreamFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("already canceled request reached upstream")
		}),
	}
	result, err := svc.Forward(context.WithoutCancel(ctx), c, newOpenAIPartialUsageAccount(true), body)
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
}

func TestPassthroughLifecycleFixedDisconnectBudget(t *testing.T) {
	for _, seconds := range []int{0, 2} {
		for _, continuous := range []bool{false, true} {
			t.Run(fmt.Sprintf("timeout_%d/continuous_%t", seconds, continuous), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					c, recorder := newOpenAIPartialUsageContext(t, nil)
					c.Request = c.Request.WithContext(ctx)
					resp, tracked, pw := newLifecycleTrackingResponse()
					defer pw.Close()
					svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: seconds}}}
					done := make(chan error, 1)
					go func() {
						_, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, newOpenAIPartialUsageAccount(true), time.Now(), "gpt-5.4", "gpt-5.4")
						done <- err
					}()
					_, err := io.WriteString(pw, partialOpenAIResponsesSSE(false)+"\n")
					require.NoError(t, err)
					synctest.Wait()
					cancel()
					synctest.Wait()
					budget := openAIStreamClientDisconnectDrainTimeoutDefault
					if seconds > 0 {
						budget = time.Duration(seconds) * time.Second
					}
					if continuous {
						go func() {
							for {
								select {
								case <-tracked.closed:
									return
								case <-time.After(200 * time.Millisecond):
								}
								if _, err := io.WriteString(pw, ": upstream heartbeat\n\n"); err != nil {
									return
								}
							}
						}()
					}
					time.Sleep(budget - time.Nanosecond)
					select {
					case err := <-done:
						t.Fatalf("early drain exit: %v", err)
					default:
					}
					time.Sleep(time.Nanosecond)
					synctest.Wait()
					select {
					case err := <-done:
						require.ErrorContains(t, err, "disconnect timeout")
					default:
						t.Fatal("upstream activity extended disconnect budget")
					}
					require.Empty(t, recorder.Body.String())
					select {
					case <-tracked.closed:
					default:
						t.Fatal("source body remained open")
					}
				})
			})
		}
	}
}

func TestPassthroughLifecycleDownstreamFailureStillCollectsUsage(t *testing.T) {
	for _, cause := range []string{"cancel", "body_heartbeat", "output", "header_heartbeat"} {
		t.Run(cause, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, recorder := newOpenAIPartialUsageContext(t, nil)
				c.Request = c.Request.WithContext(ctx)
				writer := &lifecycleFailedClientWriter{ResponseWriter: c.Writer, failed: make(chan struct{})}
				if cause != "cancel" {
					c.Writer = writer
				}
				stop := StartOpenAIStreamSSEKeepalive(c, time.Second)
				defer stop()
				if cause == "header_heartbeat" {
					time.Sleep(time.Second)
					synctest.Wait()
				}
				resp, _, pw := newLifecycleTrackingResponse()
				defer pw.Close()
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 3, StreamKeepaliveInterval: 1}}}
				done := make(chan *openaiStreamingResultPassthrough, 1)
				errCh := make(chan error, 1)
				go func() {
					result, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, newOpenAIPartialUsageAccount(true), time.Now(), "gpt-5.4", "gpt-5.4")
					done <- result
					errCh <- err
				}()
				_, err := io.WriteString(pw, partialOpenAIResponsesSSE(cause == "output")+"\n")
				require.NoError(t, err)
				synctest.Wait()
				if cause == "cancel" {
					cancel()
				} else if cause == "body_heartbeat" {
					time.Sleep(time.Second)
				}
				synctest.Wait()
				time.Sleep(500 * time.Millisecond)
				_, err = io.WriteString(pw, "data: "+timingReviewTerminal+"\n\n")
				require.NoError(t, err)
				synctest.Wait()
				result := <-done
				require.NoError(t, <-errCh)
				require.True(t, result.clientDisconnect)
				require.Equal(t, 12, result.usage.InputTokens)
				require.Equal(t, 4, result.usage.OutputTokens)
				require.Equal(t, 3, result.usage.CacheReadInputTokens)
				require.Empty(t, recorder.Body.String())
			})
		})
	}
}

func TestPassthroughLifecycleHeartbeatHandoffAndReadIdle(t *testing.T) {
	for _, mode := range []string{"silent", "active", "partial_line", "partial_event"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, recorder := newOpenAIPartialUsageContext(t, nil)
				stop := StartOpenAIStreamSSEKeepalive(c, time.Second)
				defer stop()
				resp, _, pw := newLifecycleTrackingResponse()
				defer pw.Close()
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 2, StreamKeepaliveInterval: 1}}}
				done := make(chan error, 1)
				go func() {
					_, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, newOpenAIPartialUsageAccount(true), time.Now(), "gpt-5.4", "gpt-5.4")
					done <- err
				}()
				synctest.Wait()
				time.Sleep(time.Second)
				synctest.Wait()
				require.Contains(t, recorder.Body.String(), ": keepalive\n\n", "body keepalive must survive an installed header manager")
				require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
				if mode == "silent" {
					time.Sleep(time.Second)
					synctest.Wait()
					err := <-done
					var failover *UpstreamFailoverError
					require.True(t, errors.As(err, &failover), "heartbeat-only output still permits failover")
					return
				}
				if mode == "partial_event" {
					_, err := io.WriteString(pw, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n")
					require.NoError(t, err)
					synctest.Wait()
					before := recorder.Body.String()
					time.Sleep(time.Second)
					synctest.Wait()
					require.Equal(t, before, recorder.Body.String(), "heartbeat must not dispatch an unfinished event")
					_, err = io.WriteString(pw, "\n")
					require.NoError(t, err)
				} else {
					for range 5 {
						frame := ": upstream\n\n"
						if mode == "partial_line" {
							frame = " "
						}
						_, err := io.WriteString(pw, frame)
						require.NoError(t, err)
						time.Sleep(time.Second)
						synctest.Wait()
						select {
						case err := <-done:
							t.Fatalf("healthy stream timed out: %v", err)
						default:
						}
					}
					if mode == "partial_line" {
						_, err := io.WriteString(pw, "\n\n")
						require.NoError(t, err)
					}
				}
				_, err := io.WriteString(pw, "data: "+timingReviewTerminal+"\n\n")
				require.NoError(t, err)
				synctest.Wait()
				require.NoError(t, <-done)
				require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.completed"`))
			})
		})
	}
}

func TestPassthroughLifecyclePartialOutputIdleNeverReplays(t *testing.T) {
	for _, output := range []string{
		`{"type":"response.output_text.delta","delta":"partial text","usage":{"input_tokens":13,"output_tokens":4}}`,
		`{"type":"response.function_call_arguments.delta","delta":"partial tool","usage":{"input_tokens":13,"output_tokens":4}}`,
	} {
		for _, boundary := range []string{"\n", "\n\n"} {
			synctest.Test(t, func(t *testing.T) {
				c, recorder := newOpenAIPartialUsageContext(t, nil)
				resp, _, pw := newLifecycleTrackingResponse()
				defer pw.Close()
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}}
				done := make(chan *openaiStreamingResultPassthrough, 1)
				errorsCh := make(chan error, 1)
				go func() {
					result, err := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, newOpenAIPartialUsageAccount(true), time.Now(), "gpt-5.4", "gpt-5.4")
					done <- result
					errorsCh <- err
				}()
				_, err := io.WriteString(pw, "data: "+output+boundary)
				require.NoError(t, err)
				synctest.Wait()
				time.Sleep(time.Second)
				synctest.Wait()
				result, err := <-done, <-errorsCh
				require.ErrorContains(t, err, "upstream response failed:")
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.Equal(t, 13, result.usage.InputTokens)
				require.Equal(t, 4, result.usage.OutputTokens)
				require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.failed"`))
				require.Equal(t, 1, strings.Count(recorder.Body.String(), output))
				require.True(t, strings.HasSuffix(recorder.Body.String(), "\n\n"))
			})
		}
	}
}

func TestPassthroughLifecycleClientToolRawReadActivity(t *testing.T) {
	for _, completes := range []bool{false, true} {
		t.Run(fmt.Sprintf("completes_%t", completes), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				body := openAIClientToolsRequest(true)
				c, recorder := newOpenAIPartialUsageContext(t, body)
				resp, tracked, pw := newLifecycleTrackingResponse()
				resp.Header.Set("Content-Type", "text/event-stream")
				defer pw.Close()
				calls := 0
				var upstreamBody []byte
				svc := &OpenAIGatewayService{
					cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}},
					httpUpstream: headerCancelUpstreamFunc(func(req *http.Request) (*http.Response, error) {
						calls++
						var err error
						upstreamBody, err = io.ReadAll(req.Body)
						if err != nil {
							return nil, err
						}
						return resp, nil
					}),
				}
				done := make(chan headerCancelForwardResult, 1)
				go func() {
					result, err := svc.Forward(context.Background(), c, newOpenAIPartialUsageAccount(true), body)
					done <- headerCancelForwardResult{result, err}
				}()
				// The restorer buffers this entire JSON line while actual raw reads
				// continue for longer than the idle interval.
				for _, fragment := range []string{
					`data: {"type":"response.output_item.added"`,
					`,"sequence_number":0,"output_index":0`,
					`,"item":{"type":"function_call","id":"item_raw_activity"`,
					`,"call_id":"call_raw_activity","name":"exec"`,
				} {
					_, err := io.WriteString(pw, fragment)
					require.NoError(t, err)
					time.Sleep(600 * time.Millisecond)
					synctest.Wait()
					select {
					case got := <-done:
						t.Fatalf("raw bytes were arriving but transformed stream timed out: %v", got.err)
					default:
					}
				}
				if !completes {
					time.Sleep(time.Second)
					synctest.Wait()
					got := <-done
					var failover *UpstreamFailoverError
					require.ErrorAs(t, got.err, &failover, "raw activity must stop extending idle once bytes stop")
					require.Contains(t, string(failover.ResponseBody), "data interval timeout")
					require.Empty(t, recorder.Body.String())
				} else {
					_, err := io.WriteString(pw, ",\"status\":\"in_progress\"}}\n\n")
					require.NoError(t, err)
					_, err = io.WriteString(pw, "data: "+timingReviewTerminal+"\n\n")
					require.NoError(t, err)
					synctest.Wait()
					got := <-done
					require.NoError(t, got.err)
					require.Equal(t, 12, got.result.Usage.InputTokens)
					require.Equal(t, 4, got.result.Usage.OutputTokens)
					require.Equal(t, 3, got.result.Usage.CacheReadInputTokens)
					require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"custom_tool_call"`))
					require.Equal(t, 1, strings.Count(recorder.Body.String(), `"type":"response.completed"`))
					require.NotContains(t, recorder.Body.String(), `"type":"response.failed"`)
				}
				require.Equal(t, 1, calls)
				assertOpenAIClientToolsLowered(t, upstreamBody)
				select {
				case <-tracked.closed:
				default:
					t.Fatal("raw source body remained open")
				}
			})
		})
	}
}

func TestPassthroughLifecycleTerminalIdleCompletesEventBoundary(t *testing.T) {
	for _, eventType := range []string{"response.completed", "response.failed"} {
		t.Run(eventType, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
				recorder := newOpenAIResponseFlushRecorder()
				c, _ := gin.CreateTestContext(recorder)
				requestContext, _ := newOpenAIPartialUsageContext(t, body)
				c.Request = requestContext.Request
				resp, tracked, pw := newLifecycleTrackingResponse()
				resp.Header.Set("Content-Type", "text/event-stream")
				defer pw.Close()
				calls := 0
				svc := &OpenAIGatewayService{
					cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1, StreamKeepaliveInterval: 1}},
					httpUpstream: headerCancelUpstreamFunc(func(*http.Request) (*http.Response, error) {
						calls++
						return resp, nil
					}),
				}
				done := make(chan headerCancelForwardResult, 1)
				go func() {
					result, err := svc.Forward(context.Background(), c, newOpenAIPartialUsageAccount(true), body)
					done <- headerCancelForwardResult{result, err}
				}()
				// Commit partial text and tool output, then withhold only the final
				// terminal event's blank line while keeping the source body open.
				preamble := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial text\"}\n\n" +
					"data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"partial tool\"}\n\n"
				terminal := timingReviewTerminal
				if eventType == "response.failed" {
					terminal = `{"type":"response.failed","response":{"id":"resp_timing","status":"failed","error":{"code":"stream_error","message":"upstream test failure"},"usage":{"input_tokens":12,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}}}`
				}
				_, err := io.WriteString(pw, preamble+"event: "+eventType+"\ndata: "+terminal+"\n")
				require.NoError(t, err)
				synctest.Wait()
				_, flushes := recorder.snapshot()
				require.Len(t, flushes, 2, "terminal must wait for a boundary until idle expires")
				time.Sleep(time.Second)
				synctest.Wait()
				got := <-done
				if eventType == "response.completed" {
					require.NoError(t, got.err)
				} else {
					require.EqualError(t, got.err, "upstream response failed: upstream test failure")
				}
				var failover *UpstreamFailoverError
				require.False(t, errors.As(got.err, &failover))
				require.NotNil(t, got.result)
				require.Equal(t, 12, got.result.Usage.InputTokens)
				require.Equal(t, 4, got.result.Usage.OutputTokens)
				require.Equal(t, 3, got.result.Usage.CacheReadInputTokens)
				output, flushes := recorder.snapshot()
				require.True(t, strings.HasSuffix(output, "\n\n"), "client SSE parser must be able to dispatch the terminal")
				require.Equal(t, output, flushes[len(flushes)-1])
				require.Equal(t, 1, strings.Count(output, `"type":"`+eventType+`"`))
				require.Equal(t, 1, strings.Count(output, `"delta":"partial text"`))
				require.Equal(t, 1, strings.Count(output, `"delta":"partial tool"`))
				require.Equal(t, 1, calls)
				select {
				case <-tracked.closed:
				default:
					t.Fatal("terminal source body remained open")
				}
			})
		})
	}
}

func TestPassthroughLifecycleTerminalEOFKeepsFraming(t *testing.T) {
	for _, eventType := range []string{"response.completed", "response.failed"} {
		t.Run(eventType, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
			c, recorder := newOpenAIPartialUsageContext(t, body)
			terminal := timingReviewTerminal
			if eventType == "response.failed" {
				terminal = `{"type":"response.failed","response":{"id":"resp_timing","status":"failed","error":{"code":"stream_error","message":"upstream test failure"},"usage":{"input_tokens":12,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}}}`
			}
			preamble := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			frame := preamble + "data: " + terminal + "\n"
			wantFrame := frame
			if eventType == "response.failed" {
				// Existing client sanitization removes failed response usage;
				// the internal result must still retain it for billing below.
				wantFrame = preamble + "data: " + `{"type":"response.failed","response":{"id":"resp_timing","status":"failed","error":{"code":"stream_error","message":"upstream test failure"}}}` + "\n"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(frame)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}, httpUpstream: upstream}
			result, err := svc.Forward(context.Background(), c, newOpenAIPartialUsageAccount(true), body)
			if eventType == "response.completed" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, "upstream response failed: upstream test failure")
			}
			require.NotNil(t, result)
			require.Equal(t, 12, result.Usage.InputTokens)
			require.Equal(t, 4, result.Usage.OutputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
			require.Equal(t, wantFrame, recorder.Body.String(), "idle repair must preserve existing EOF framing")
		})
	}
}

type passthroughCloseIgnoringBody struct{ release <-chan struct{} }

func (b passthroughCloseIgnoringBody) Read([]byte) (int, error) { <-b.release; return 0, io.EOF }
func (b passthroughCloseIgnoringBody) Close() error             { return nil }

func TestPassthroughLifecycleScannerShutdownBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		_, events, stop := startOpenAIPassthroughScanner(passthroughCloseIgnoringBody{release: release}, defaultMaxLineSize)
		synctest.Wait()
		started := time.Now()
		stop()
		require.Equal(t, openAIStreamScannerShutdownTimeout, time.Since(started))
		// A source ignoring Close may outlive the request, but the scanner must
		// retain its buffer and only release it when the blocked read completes.
		close(release)
		synctest.Wait()
		_, open := <-events
		require.False(t, open)
	})
}
