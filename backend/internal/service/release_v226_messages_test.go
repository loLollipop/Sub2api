//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestV226MessagesSyncJSONAndSSE(t *testing.T) {
	for _, status := range []string{"completed", "incomplete", "failed"} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sse=%v", status, sse), func(t *testing.T) {
				body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":false}`)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
				payload := fmt.Sprintf(`{"id":"resp_sync","object":"response","status":%q,"model":"gpt-5.4","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":17,"output_tokens":3},"error":{"code":"invalid_prompt","message":"Content policy violation"}}`, status)
				contentType := "application/json"
				if sse {
					contentType = "text/event-stream"
					payload = fmt.Sprintf("data: {\"type\":\"response.%s\",\"response\":%s}\n\n", status, payload)
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				result, err := svc.ForwardAsAnthropic(context.Background(), c, rawChatCompletionsTestAccount(), body, "", "")
				require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool(), "sync clients must request sync upstream")
				if status == "failed" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), "answer")
					require.Equal(t, "gpt-5.4", result.UpstreamResponseModel)
				}
				require.NotNil(t, result)
				require.Equal(t, 17, result.Usage.InputTokens)
				require.Equal(t, 3, result.Usage.OutputTokens)
				require.False(t, result.Stream)
			})
		}
	}
}

func TestV226MessagesSyncJSONReadBound(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.UpstreamResponseReadMaxBytes = 16
	svc := &OpenAIGatewayService{cfg: cfg}
	_, _, _, err := svc.readAnthropicCompatBufferedResponse(&http.Response{Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 17)))}, c, "")
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestV226MessagesSyncReadDeadline(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, payload string
		cancel                     bool
		seconds                    int
	}{
		{name: "silent JSON", contentType: "application/json", seconds: 1},
		{name: "silent missing content type", seconds: 1},
		{name: "valid JSON without EOF", contentType: "application/json", payload: `{"id":"resp_sync","status":"completed","output":[],"usage":{"input_tokens":17}}`, seconds: 1},
		{name: "canceled client", contentType: "application/json", cancel: true, seconds: 1},
		{name: "default deadline", contentType: "application/json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, _ := newLifecycleTestContext(ctx)
				resp, tracked, pw := newLifecycleTrackingResponse()
				defer func() { _ = pw.Close() }()
				resp.Header.Set("Content-Type", tc.contentType)
				cfg := rawChatCompletionsTestConfig()
				cfg.Gateway.StreamDataIntervalTimeout = tc.seconds
				svc := &OpenAIGatewayService{cfg: cfg}
				done := make(chan error, 1)
				go func() {
					_, _, _, err := svc.readAnthropicCompatBufferedResponse(resp, c, "")
					done <- err
				}()
				<-tracked.readStarted
				if tc.cancel {
					cancel()
				}
				if tc.payload != "" {
					_, err := io.WriteString(pw, tc.payload)
					require.NoError(t, err)
				}
				budget := openAIStreamClientDisconnectDrainTimeoutDefault
				if tc.seconds > 0 {
					budget = time.Duration(tc.seconds) * time.Second
				}
				time.Sleep(budget / 2)
				select {
				case err := <-done:
					t.Fatalf("reader closed before its response/drain budget: %v", err)
				default:
				}
				time.Sleep(budget)
				synctest.Wait()
				select {
				case err := <-done:
					if tc.cancel {
						require.ErrorIs(t, err, context.Canceled)
					} else {
						require.ErrorIs(t, err, context.DeadlineExceeded)
					}
				case <-time.After(time.Nanosecond):
					_ = tracked.Close()
					<-done
					t.Fatal("buffered response stayed blocked beyond its finite deadline")
				}
				select {
				case <-tracked.closed:
				default:
					t.Fatal("deadline did not close the upstream body")
				}
			})
		})
	}
}

func TestV226MessagesMissingContentTypeWhitespace(t *testing.T) {
	for _, sse := range []bool{false, true} {
		payload := `{"id":"resp_sync","status":"completed","output":[],"usage":{"input_tokens":17}}`
		if sse {
			payload = "data: {\"type\":\"response.completed\",\"response\":" + payload + "}\n\n"
		}
		c, _ := newLifecycleTestContext(context.Background())
		svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
		response, usage, _, err := svc.readAnthropicCompatBufferedResponse(&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(" \r\n\t" + payload))}, c, "")
		require.NoError(t, err)
		require.NotNil(t, response)
		require.Equal(t, 17, usage.InputTokens)
	}
	c, _ := newLifecycleTestContext(context.Background())
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	_, _, _, err := svc.readAnthropicCompatBufferedResponse(&http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", 4096)))}, c, "")
	require.ErrorContains(t, err, "sniff limit exceeded")
}

func TestV226MessagesSyncCancellationDrainsUsage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c, _ := newLifecycleTestContext(ctx)
		resp, tracked, pw := newLifecycleTrackingResponse()
		resp.Header.Set("Content-Type", "application/json")
		cfg := rawChatCompletionsTestConfig()
		cfg.Gateway.StreamDataIntervalTimeout = 1
		svc := &OpenAIGatewayService{cfg: cfg}
		producerDone := make(chan struct{})
		go func() {
			defer close(producerDone)
			defer func() { _ = pw.Close() }()
			<-tracked.readStarted
			cancel()
			time.Sleep(500 * time.Millisecond)
			_, _ = io.WriteString(pw, `{"id":"resp_drained","status":"completed","output":[],"usage":{"input_tokens":17,"output_tokens":3}}`)
		}()
		response, usage, _, err := svc.readAnthropicCompatBufferedResponse(resp, c, "")
		<-producerDone
		require.NoError(t, err)
		require.Equal(t, "resp_drained", response.ID)
		require.Equal(t, 17, usage.InputTokens)
		require.Equal(t, 3, usage.OutputTokens)
		select {
		case <-tracked.closed:
		default:
			t.Fatal("completed drain left upstream body open")
		}
	})
}

func TestV226MessagesBufferedRefusalNoCharge(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformGrok} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sse=%v", platform, sse), func(t *testing.T) {
				c, _ := newLifecycleTestContext(context.Background())
				payload := `{"id":"resp_refused","status":"failed","output":[],"usage":{"input_tokens":17,"output_tokens":0},"error":{"code":"invalid_prompt","message":"Content policy violation"}}`
				contentType := "application/json"
				if sse {
					contentType = "text/event-stream"
					payload = "data: {\"type\":\"response.failed\",\"response\":" + payload + "}\n\n"
				}
				account := rawChatCompletionsTestAccount()
				account.Platform = platform
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
				result, err := svc.handleAnthropicBufferedStreamingResponse(&http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload))}, c, account, "gpt-5.1", "gpt-5.1", "gpt-5.1", time.Now())
				require.Error(t, err)
				require.NotNil(t, result)
				require.True(t, result.NonBillableUpstreamError, "buffered failures were previously uncharged")
				usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
				billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
				userRepo := &openAIRecordUsageUserRepoStub{}
				billingSvc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, &openAIRecordUsageSubRepoStub{}, nil)
				err = billingSvc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 1000, Group: &Group{RateMultiplier: 1}}, User: &User{ID: 2000}, Account: account})
				require.NoError(t, err)
				require.Equal(t, 17, usageRepo.lastLog.InputTokens)
				require.Zero(t, usageRepo.lastLog.TotalCost)
				require.Zero(t, usageRepo.lastLog.ActualCost)
				require.Zero(t, billingRepo.lastCmd.BalanceCost)
				require.Zero(t, userRepo.deductCalls)
			})
		}
	}
}
