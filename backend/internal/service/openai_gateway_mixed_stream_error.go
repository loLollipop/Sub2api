package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const mixedCCAccountContextKey = "mixed_cc_error_account"
const mixedCCTargetContextKey = "mixed_cc_error_target"

// This error owns only a bounded diagnostic, never a provider payload. Provider
// messages can contain an entire echoed request, even inside error.message.
type mixedCCFailure struct {
	summary     string
	nonBillable bool
	cause       error
}

func (e *mixedCCFailure) Error() string { return e.summary }
func (e *mixedCCFailure) Unwrap() error { return e.cause }

func mixedCCErrorNonBillable(err error) bool {
	var failure *mixedCCFailure
	return errors.As(err, &failure) && failure.nonBillable
}

func mixedCCAccount(c *gin.Context) *Account {
	if c == nil {
		return nil
	}
	value, _ := c.Get(mixedCCAccountContextKey)
	account, _ := value.(*Account)
	return account
}

func mixedCCSafeText(c *gin.Context, value string, limit int) string {
	if account := mixedCCAccount(c); account != nil {
		if secret := strings.TrimSpace(account.GetCredential("api_key")); secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	value = logredact.RedactText(value, "api_key", "authorization", "token", "secret", "prompt", "input", "messages", "content")
	return truncateString(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value), limit)
}

// Only vocabulary with a defined diagnostic meaning may survive. Unknown
// identifiers get a fingerprint so an echoed prompt cannot become a log field.
func mixedCCSafeIdentifier(value string, allowed string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	for _, candidate := range strings.Fields(allowed) {
		if value == candidate {
			return value
		}
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("redacted:%x", sum[:6])
}

func newMixedCCFailure(c *gin.Context, payload []byte, event, kind string, outputStarted bool, cause error) *mixedCCFailure {
	valid := gjson.ValidBytes(payload)
	message, code, param := "", "", ""
	if valid {
		message = firstValidTrimmedGJSONString(payload, "response.error.message", "error.message", "message")
		code = firstValidTrimmedGJSONString(payload, "response.error.code", "error.code", "code", "error.type")
		param = firstValidTrimmedGJSONString(payload, "response.error.param", "error.param", "param")
		if event == "" {
			event = firstValidTrimmedGJSONString(payload, "type")
		}
	}
	policy := mixedCCPayloadHasPolicy(payload)
	nonBillable := kind == "upstream_error" && valid && !outputStarted && !policy && isOpenAINonBillableRequestError(message, payload)
	// Save a classification of the message, not arbitrary provider prose.
	messageClass := "redacted upstream message"
	lower := strings.ToLower(message + " " + code)
	switch {
	case policy:
		messageClass = "policy refusal"
	case isOpenAIContextWindowError(message, payload) && valid:
		messageClass = "context window exceeded"
	case nonBillable:
		messageClass = "invalid thinking budget"
	case strings.Contains(lower, "unsupported") || strings.Contains(lower, "unknown parameter"):
		messageClass = "unsupported request parameter"
	case strings.Contains(lower, "quota") || strings.Contains(lower, "balance") || strings.Contains(lower, "billing"):
		messageClass = "upstream account unavailable"
	case strings.Contains(lower, "rate_limit") || strings.Contains(lower, "rate limit"):
		messageClass = "upstream rate limited"
	case strings.Contains(lower, "overload") || strings.Contains(lower, "server_error"):
		messageClass = "upstream server error"
	case kind != "upstream_error":
		messageClass = kind
	}
	// Redact untrusted fields before mapping them to diagnostic vocabulary.
	// Redacting the assembled summary would also erase the harmless error code
	// because logredact treats the generic key "code" as an OAuth secret.
	code = mixedCCSafeIdentifier(mixedCCSafeText(c, code, 256), "invalid_request_error invalid_parameter unsupported_parameter unknown_parameter context_length_exceeded context_window_exceeded insufficient_quota billing_not_active rate_limit_exceeded server_error api_error model_not_found invalid_api_key content_policy_violation content_filter cyber_policy")
	param = mixedCCSafeIdentifier(mixedCCSafeText(c, param, 256), "model messages input tools tool_choice response_format reasoning reasoning_effort thinking thinking.budget_tokens budget_tokens max_tokens max_completion_tokens stream stream_options temperature top_p parallel_tool_calls")
	event = mixedCCSafeIdentifier(mixedCCSafeText(c, event, 256), "error response.failed response.incomplete")
	summary := fmt.Sprintf("Mixed Chat Completions %s: event=%s code=%s param=%s message=%s endpoint=/v1/chat/completions", kind, event, code, param, messageClass)
	return &mixedCCFailure{summary: summary, nonBillable: nonBillable, cause: cause}
}

func mixedCCPayloadHasPolicy(payload []byte) bool {
	if !gjson.ValidBytes(payload) {
		return false
	}
	if hit, _, _ := detectOpenAICyberPolicy(payload); hit {
		return true
	}
	for _, choice := range gjson.GetBytes(payload, "choices").Array() {
		if choice.Get("delta.refusal").String() != "" || choice.Get("message.refusal").String() != "" || choice.Get("finish_reason").String() == "content_filter" {
			return true
		}
	}
	classification := strings.ToLower(firstValidTrimmedGJSONString(payload, "response.error.code", "error.code", "code") + " " + firstValidTrimmedGJSONString(payload, "response.error.type", "error.type"))
	for _, marker := range []string{"policy", "safety", "content_filter", "refusal"} {
		if strings.Contains(classification, marker) {
			return true
		}
	}
	return false
}

func mixedCCPayloadHasOutput(payload []byte) bool {
	for _, choice := range gjson.GetBytes(payload, "choices").Array() {
		for _, field := range []string{"delta", "message"} {
			part := choice.Get(field)
			if part.Get("content").String() != "" || part.Get("refusal").String() != "" || part.Get("reasoning_content").String() != "" || part.Get("reasoning").String() != "" || part.Get("reasoning_summary").String() != "" {
				return true
			}
			if function := part.Get("function_call"); function.Get("name").String() != "" || function.Get("arguments").String() != "" {
				return true
			}
			for _, tool := range part.Get("tool_calls").Array() {
				if tool.Get("id").String() != "" || tool.Get("function.name").String() != "" || tool.Get("function.arguments").String() != "" {
					return true
				}
			}
		}
	}
	return false
}

func mixedCCPayloadIsError(payload []byte, event string) bool {
	return mixedPayloadIsError(payload, event) || gjson.GetBytes(payload, "type").String() == "response.failed" || gjson.GetBytes(payload, "response.error").IsObject()
}

// Raw Chat preserves these native fields, but the existing Responses/Messages
// converters do not implement them. Fail explicitly instead of dropping a tool
// call or refusal and synthesizing a successful empty completion.
func mixedCCUnsupportedConversionOutput(payload []byte) bool {
	for _, choice := range gjson.GetBytes(payload, "choices").Array() {
		for _, field := range []string{"delta", "message"} {
			part := choice.Get(field)
			if part.Get("refusal").String() != "" || part.Get("reasoning_summary").String() != "" {
				return true
			}
			if function := part.Get("function_call"); function.Get("name").String() != "" || function.Get("arguments").String() != "" {
				return true
			}
		}
	}
	return false
}

func observeMixedCCPayload(c *gin.Context, payload []byte, eventType ...string) {
	if !gjson.ValidBytes(payload) {
		return
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	event := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if len(eventType) > 0 && eventType[0] != "" {
		event = eventType[0]
	}
	observer.ObserveOpenAI(payload, event)
}

// Both raw and converted buffered paths must keep metering and attribution
// received before a read failure or a HTTP-200 provider error.
func (s *OpenAIGatewayService) readMixedCCResponse(c *gin.Context, resp *http.Response) ([]byte, OpenAIUsage, error) {
	body, err := s.readMixedUpstreamBody(resp)
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	observeMixedCCPayload(c, body)
	if err == nil && (mixedCCBufferedIncomplete(body) || mixedCCPayloadIsError(body, "")) {
		kind := "incomplete_response"
		if !gjson.ValidBytes(body) {
			kind = "invalid_json"
		} else if mixedCCPayloadIsError(body, "") {
			kind = "upstream_error"
		}
		err = newMixedCCFailure(c, body, "", kind, mixedCCPayloadHasOutput(body) || mixedCCPayloadHasPolicy(body), nil)
	}
	if err != nil {
		err = s.recordMixedCCFailure(c, resp, err)
	}
	return body, usage, err
}

func (s *OpenAIGatewayService) recordMixedCCFailure(c *gin.Context, resp *http.Response, err error) error {
	var failure *mixedCCFailure
	if !errors.As(err, &failure) {
		kind := "read_error"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			kind = "cancelled"
		} else if errors.Is(err, ErrOpenAIUpstreamStreamTruncated) {
			kind = "stream_truncated"
		}
		failure = newMixedCCFailure(c, nil, "", kind, false, err)
	}
	// Pass only the safe summary to the shared Ops recorder. In particular,
	// enabling raw error logging must never persist the original SSE payload.
	s.recordOpenAIStreamUpstreamError(c, mixedCCAccount(c), false, mixedCCSafeText(c, resp.Header.Get("x-request-id"), 128), "stream_failed", nil, failure.summary)
	if c != nil {
		if value, ok := c.Get(OpsUpstreamErrorsKey); ok {
			if events, ok := value.([]*OpsUpstreamErrorEvent); ok && len(events) > 0 {
				event := events[len(events)-1]
				if target, err := url.Parse(c.GetString(mixedCCTargetContextKey)); err == nil {
					// Endpoint attribution never includes URL credentials or query.
					event.UpstreamURL = mixedCCSafeText(c, target.Scheme+"://"+target.Host+target.Path, 256)
				}
			}
		}
	}
	return failure
}
