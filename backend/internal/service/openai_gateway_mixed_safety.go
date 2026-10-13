package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Mixed has no disconnected-client drain/replay budget. Bind every header and
// body read to the original client, even when a legacy builder detached it.
func (s *OpenAIGatewayService) doOpenAITextUpstream(ctx context.Context, c *gin.Context, request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if !account.IsMixed() {
		return s.doOpenAIUpstream(request, proxyURL, account)
	}
	clientCtx := ctx
	if c != nil && c.Request != nil {
		clientCtx = c.Request.Context()
	}
	upstreamCtx, cancel := context.WithCancel(WithHTTPUpstreamProfile(WithHTTPUpstreamRedirectsDisabled(ctx), HTTPUpstreamProfileOpenAI))
	stopClient := context.AfterFunc(clientCtx, cancel)
	if clientCtx.Err() != nil {
		cancel()
	}
	resp, err := s.doOpenAIUpstream(request.WithContext(upstreamCtx), proxyURL, account)
	if err != nil || upstreamCtx.Err() != nil {
		stopClient()
		cancel()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if clientCtx.Err() != nil {
			return nil, clientCtx.Err()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	body := &mixedCancelableBody{ReadCloser: resp.Body, ctx: upstreamCtx, cancel: cancel, stopClient: stopClient}
	context.AfterFunc(upstreamCtx, func() { _ = body.Close() })
	resp.Body = body
	return resp, nil
}

type mixedCancelableBody struct {
	io.ReadCloser
	ctx        context.Context
	cancel     context.CancelFunc
	stopClient func() bool
	once       sync.Once
}

func (b *mixedCancelableBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if b.ctx.Err() != nil {
		err = b.ctx.Err()
	}
	return n, err
}

func (b *mixedCancelableBody) Close() error {
	var err error
	b.once.Do(func() {
		b.stopClient()
		err = b.ReadCloser.Close()
		b.cancel()
	})
	return err
}

// Complete explicit refusal evidence is required before replay. Unlike legacy
// diagnostic reads, Mixed propagates read errors and rejects oversized bodies.
func (s *OpenAIGatewayService) readOpenAIProtocolError(resp *http.Response, account *Account) ([]byte, string, error) {
	if !account.IsMixed() {
		body, msg := s.readOpenAIUpstreamError(resp)
		return body, msg, nil
	}
	limit := openAIUpstreamErrorBodyReadLimitForConfig(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("read upstream error body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("upstream error body exceeds refusal read limit")
	}
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	return body, sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body))), nil
}

func writeMixedStreamFailure(c *gin.Context, protocol string, cause error) {
	if c.Request.Context().Err() != nil {
		return
	}
	message := "Upstream stream failed before completion"
	if !c.Writer.Written() {
		if protocol == APIProtocolAnthropic {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", message)
		} else {
			writeChatCompletionsError(c, http.StatusBadGateway, "api_error", message)
		}
		return
	}
	payload := map[string]any{"error": map[string]string{"type": "api_error", "message": message}}
	prefix := ""
	if protocol == APIProtocolAnthropic {
		payload["type"] = "error"
		prefix = "event: error\n"
	} else if protocol == APIProtocolResponses {
		payload["type"] = "error"
		payload["code"] = "upstream_error"
		payload["message"] = message
		prefix = "event: error\n"
	}
	body, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(c.Writer, "%sdata: %s\n\n", prefix, body)
	c.Writer.Flush()
}

func mixedPartialForwardResult(c *gin.Context, resp *http.Response, usage OpenAIUsage, originalModel, billingModel, upstreamModel string, stream bool, startTime time.Time) *OpenAIForwardResult {
	return &OpenAIForwardResult{
		RequestID: resp.Header.Get("x-request-id"), UpstreamHeaders: resp.Header,
		Usage: usage, Model: originalModel, BillingModel: billingModel, UpstreamModel: upstreamModel,
		UpstreamResponseModel: observedUpstreamResponseModel(c), UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c), Stream: stream, Duration: time.Since(startTime),
	}
}

// The shared legacy body reader discards partial bytes on a read error. Mixed
// must retain bytes already read so observed usage can accompany the error.
func (s *OpenAIGatewayService) readMixedUpstreamBody(resp *http.Response) ([]byte, error) {
	limit := resolveUpstreamResponseReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(body)) > limit {
		return body[:limit], ErrUpstreamResponseBodyTooLarge
	}
	return body, err
}

func mixedStreamReadFailure(err error) error {
	if err == nil || err == io.EOF {
		return ErrOpenAIUpstreamStreamTruncated
	}
	return fmt.Errorf("upstream stream read failed: %w", err)
}

func mixedPayloadIsError(payload []byte, eventType string) bool {
	return eventType == "error" || eventType == "response.failed" || gjson.GetBytes(payload, "type").String() == "error" ||
		gjson.GetBytes(payload, "error").IsObject() || gjson.GetBytes(payload, "response.status").String() == "failed" || gjson.GetBytes(payload, "status").String() == "failed"
}

// A terminal event name alone is not completion evidence: truncated JSON can
// still expose a best-effort type to the legacy SSE parser.
func mixedValidResponsesTerminal(payload []byte, eventType string) bool {
	if !gjson.ValidBytes(payload) {
		return false
	}
	response := gjson.GetBytes(payload, "response")
	if !response.IsObject() || !response.Get("output").IsArray() {
		return false
	}
	status := response.Get("status").String()
	switch eventType {
	case "response.completed", "response.done":
		return status == "completed" || status == "incomplete"
	case "response.incomplete":
		return status == "incomplete"
	default:
		return false
	}
}

func mixedCCBufferedIncomplete(body []byte) bool {
	if !gjson.ValidBytes(body) || mixedPayloadIsError(body, "") {
		return true
	}
	choices := gjson.GetBytes(body, "choices").Array()
	if len(choices) == 0 {
		return true
	}
	for _, choice := range choices {
		if strings.TrimSpace(choice.Get("finish_reason").String()) == "" {
			return true
		}
	}
	return false
}
