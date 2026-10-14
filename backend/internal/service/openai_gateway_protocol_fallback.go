package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// MixedLocalRequestError is a local client limitation, without any upstream
// attempt or account-health consequence.
type MixedLocalRequestError struct{}

func (MixedLocalRequestError) Error() string { return "Mixed supports text HTTP requests only" }

func IsMixedLocalRequestError(err error) bool {
	var limitation MixedLocalRequestError
	return errors.As(err, &limitation)
}

func mixedHasMediaContent(value gjson.Result) bool {
	if value.IsArray() {
		for _, item := range value.Array() {
			if mixedHasMediaContent(item) {
				return true
			}
		}
		return false
	}
	if !value.IsObject() {
		return false
	}
	switch value.Get("type").String() {
	case "image", "image_url", "input_image", "input_audio", "audio", "output_audio", "video", "video_url", "input_video", "document", "file", "input_file":
		return true
	}
	return mixedHasMediaContent(value.Get("content"))
}

func writeMixedLocalRequestError(c *gin.Context, clientProtocol string) error {
	err := MixedLocalRequestError{}
	if clientProtocol == APIProtocolAnthropic {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	} else {
		writeChatCompletionsError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	return err
}

// Protocol refusals describe this request/model, never an account capability.
// Keep the original client body and visited protocols local to one forwarding
// call so model mapping and conversion start fresh on every attempt.
type openAIProtocolFallbackKey struct{}

type openAIProtocolFallbackState struct {
	clientCtx context.Context
	visited   map[string]bool
	forced    string
	order     []string
}

type openAIProtocolRefusal struct{ next string }

func (e *openAIProtocolRefusal) Error() string {
	return "upstream explicitly refused the request protocol"
}

func openAIProtocolFallbackFromContext(ctx context.Context) *openAIProtocolFallbackState {
	state, _ := ctx.Value(openAIProtocolFallbackKey{}).(*openAIProtocolFallbackState)
	return state
}

func genericOpenAIProtocolFallbackAccount(account *Account) bool {
	return account.IsMixed() && account.Type == AccountTypeAPIKey
}

func openAIProtocolFallbackEligible(c *gin.Context, account *Account, body []byte) bool {
	if !genericOpenAIProtocolFallbackAccount(account) || c == nil || c.Request == nil ||
		c.Request.Method != http.MethodPost ||
		GetOpenAIClientTransport(c) == OpenAIClientTransportWS || isOpenAIResponsesCompactPath(c) ||
		strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return false
	}
	path := strings.TrimRight(c.Request.URL.Path, "/")
	if !strings.HasSuffix(path, "/responses") && !strings.HasSuffix(path, "/chat/completions") && !strings.HasSuffix(path, "/messages") {
		return false
	}
	model := account.GetMappedModel(gjson.GetBytes(body, "model").String())
	if IsImageGenerationIntent(c.Request.URL.Path, model, body) || gjson.GetBytes(body, "context_management").Exists() || gjson.GetBytes(body, "audio").Exists() || gjson.GetBytes(body, "web_search_options").Exists() {
		return false
	}
	for _, modality := range gjson.GetBytes(body, "modalities").Array() {
		if modality.String() != "text" {
			return false
		}
	}
	for _, tool := range gjson.GetBytes(body, "tools").Array() {
		switch tool.Get("type").String() {
		case "image_generation", "web_search", "web_search_preview":
			return false
		}
		if strings.HasPrefix(tool.Get("type").String(), "web_search_") {
			return false
		}
	}
	for _, field := range []string{"messages", "input", "system"} {
		if mixedHasMediaContent(gjson.GetBytes(body, field)) {
			return false
		}
	}
	return true
}

// Explicit text evidence is required for all new transitions. A status or
// model_not_available code alone says nothing about a model's wire protocol.
func protocolRefusalPattern(name string) *regexp.Regexp {
	name = `(?:the\s+)?(?:openai[\s_]+)?(?:` + name + `)(?:\s+(?:api|protocol|endpoint))?`
	end := `(?:\s*[.,:;!]|\s*$|\s+(?:for|by|on)\b)`
	return regexp.MustCompile(`(?:does not support|doesn't support|not support|unsupported)\s+` + name + end + `|` + name + `\s+(?:is\s+)?(?:not supported|unsupported|not implemented)` + end)
}

var (
	responsesProtocolRefusal = protocolRefusalPattern(`responses|/v1/responses|/responses`)
	chatProtocolRefusal      = protocolRefusalPattern(`chat completions|chat_completions|(?:/v1/)?chat/completions`)
	anthropicProtocolRefusal = protocolRefusalPattern(`anthropic(?:[\s_]+messages)?|messages|/v1/messages|/messages`)
)

func isExplicitOpenAIProtocolRefusal(status int, body []byte, protocol string) bool {
	if status != http.StatusBadRequest && status != http.StatusNotFound && status != http.StatusMethodNotAllowed {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if message == "" {
		// Express-style route errors may be plain text. Accept only an exact
		// explicit route refusal; an unstructured bare 404 remains terminal.
		message = strings.ToLower(strings.TrimSpace(string(body)))
		path := protocolUpstreamEndpoint(protocol)
		return message == "cannot post "+path || message == "cannot post "+strings.TrimPrefix(path, "/v1")
	}
	// A rejected parameter/schema/tool is not evidence that the wire protocol
	// is unavailable, even when the error also names that protocol.
	for _, field := range []string{"error.param", "param", "error.parameter", "parameter"} {
		if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.Null && value.String() != "" {
			return false
		}
	}
	for _, term := range []string{"parameter", "argument", "schema", "tool", "field", "model_not_found", "unknown model", "model does not exist"} {
		if strings.Contains(message, term) || strings.Contains(strings.ToLower(extractUpstreamErrorCode(body)), term) {
			return false
		}
	}
	path := protocolUpstreamEndpoint(protocol)
	if message == "cannot post "+path || message == "cannot post "+strings.TrimPrefix(path, "/v1") {
		return true
	}
	switch protocol {
	case APIProtocolResponses:
		return responsesProtocolRefusal.MatchString(message)
	case APIProtocolChatCompletions:
		return chatProtocolRefusal.MatchString(message)
	case APIProtocolAnthropic:
		return anthropicProtocolRefusal.MatchString(message)
	}
	return false
}

func (s *OpenAIGatewayService) protocolRefusalFallback(ctx context.Context, c *gin.Context, account *Account, protocol string, status int, body []byte, legacy bool) error {
	state := openAIProtocolFallbackFromContext(ctx)
	if state == nil || state.clientCtx.Err() != nil || c == nil || c.Request == nil ||
		c.Request.Context().Err() != nil || IsResponseCommitted(c) ||
		openAIStreamClientOutputStarted(c, false) || openAIStreamHeaderKeepaliveFailed(c) ||
		!isExplicitOpenAIProtocolRefusal(status, body, protocol) {
		return nil
	}
	state.visited[protocol] = true
	for _, next := range state.order {
		if state.visited[next] {
			continue
		}
		if next == APIProtocolAnthropic {
			if !crossProtocolConversionAllowedFromContext(c) {
				continue
			}
			// No alternate domain, inferred provider endpoint, or auth retry.
			if _, err := s.nativeAnthropicTargetURL(account); err != nil {
				continue
			}
		}
		return &openAIProtocolRefusal{next: next}
	}
	return nil
}

func forcedOpenAIResponsesProtocol(ctx context.Context) bool {
	state := openAIProtocolFallbackFromContext(ctx)
	return state != nil && state.forced == APIProtocolResponses
}

func (s *OpenAIGatewayService) forwardWithProtocolFallback(ctx context.Context, c *gin.Context, account *Account, body []byte, clientProtocol, promptCacheKey, defaultMappedModel string, initial func(context.Context) (*OpenAIForwardResult, error)) (*OpenAIForwardResult, error) {
	if !account.IsMixed() {
		return initial(ctx)
	}
	requestedModel := gjson.GetBytes(body, "model").String()
	upstreamModel := normalizeOpenAIModelForUpstream(account, resolveOpenAIForwardModel(account, requestedModel, defaultMappedModel))
	if err := CheckModelScope(ctx, requestedModel, upstreamModel); err != nil {
		return nil, err
	}
	if err := validateMixedAccount(account); err != nil {
		return nil, err
	}
	if !openAIProtocolFallbackEligible(c, account, body) {
		return nil, writeMixedLocalRequestError(c, clientProtocol)
	}
	// All Mixed models start with Chat, independent of legacy account-wide
	// Responses probes. Re-entering an attempt never starts a fresh budget.
	if openAIProtocolFallbackFromContext(ctx) != nil {
		return initial(ctx)
	}
	order := []string{APIProtocolChatCompletions, APIProtocolResponses, APIProtocolAnthropic}
	if clientProtocol == APIProtocolAnthropic {
		order = []string{APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses}
	}
	state := &openAIProtocolFallbackState{clientCtx: ctx, visited: make(map[string]bool, 3), order: order}
	ctx = context.WithValue(ctx, openAIProtocolFallbackKey{}, state)
	beginUpstreamResponseModelObservation(c)
	setCodexToolNameReverse(c, nil)
	protocol := APIProtocolChatCompletions
	for attempts := 0; attempts < len(order); attempts++ {
		if err := state.clientCtx.Err(); err != nil {
			return nil, err
		}
		if err := c.Request.Context().Err(); err != nil {
			return nil, err
		}
		state.forced = protocol
		state.visited[protocol] = true
		SetActualOpenAIUpstreamEndpoint(c, protocolUpstreamEndpoint(protocol))
		var result *OpenAIForwardResult
		var err error
		switch clientProtocol {
		case APIProtocolResponses:
			switch protocol {
			case APIProtocolResponses:
				result, err = s.forwardOpenAIResponses(ctx, c, account, body)
			case APIProtocolChatCompletions:
				result, err = s.forwardResponsesViaRawChatCompletions(ctx, c, account, body)
			case APIProtocolAnthropic:
				result, err = s.forwardResponsesViaNativeAnthropic(ctx, c, account, body, "")
			}
		case APIProtocolChatCompletions:
			switch protocol {
			case APIProtocolResponses:
				result, err = s.forwardOpenAIChatCompletions(ctx, c, account, body, promptCacheKey, defaultMappedModel)
			case APIProtocolChatCompletions:
				result, err = s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
			case APIProtocolAnthropic:
				result, err = s.forwardChatCompletionsViaNativeAnthropic(ctx, c, account, body, defaultMappedModel)
			}
		case APIProtocolAnthropic:
			if !crossProtocolConversionAllowedFromContext(c) {
				return nil, CrossProtocolConversionError{}
			}
			switch protocol {
			case APIProtocolResponses:
				result, err = s.forwardOpenAIAnthropic(ctx, c, account, body, promptCacheKey, defaultMappedModel)
			case APIProtocolChatCompletions:
				result, err = s.forwardAnthropicViaRawChatCompletions(ctx, c, account, body, defaultMappedModel)
			case APIProtocolAnthropic:
				result, err = s.forwardAnthropicViaNativeAnthropicEndpoint(ctx, c, account, body, defaultMappedModel)
			}
		}
		SetActualOpenAIUpstreamEndpoint(c, protocolUpstreamEndpoint(protocol))
		if errors.Is(err, ErrModelScopeDenied) {
			return result, err
		}
		if clientErr := c.Request.Context().Err(); clientErr != nil {
			if result != nil {
				result.ClientDisconnect = true
			}
			return result, clientErr
		}
		if clientErr := state.clientCtx.Err(); clientErr != nil {
			if result != nil {
				result.ClientDisconnect = true
			}
			return result, clientErr
		}
		var refusal *openAIProtocolRefusal
		if !errors.As(err, &refusal) {
			return result, err
		}
		protocol = refusal.next
	}
	return nil, errors.New("mixed protocol attempts exhausted")
}

func protocolUpstreamEndpoint(protocol string) string {
	switch protocol {
	case APIProtocolChatCompletions:
		return "/v1/chat/completions"
	case APIProtocolAnthropic:
		return "/v1/messages"
	default:
		return "/v1/responses"
	}
}

// Generic credentials reuse their configured origin for Messages. Explicit
// existing protocol bases may change paths, but may not move the secret to a
// different origin. CN/adaptive base resolution stays in the account helpers.
func genericOpenAIAnthropicBaseURL(account *Account) (string, error) {
	base := strings.TrimSpace(account.GetCredential("base_url"))
	configured, err := url.Parse(base)
	if err != nil || configured.Host == "" || configured.User != nil {
		return "", errors.New("missing or invalid configured base_url for anthropic fallback")
	}
	selected := base
	if bases, ok := account.Credentials["api_base_urls"].(map[string]any); ok {
		if value, ok := bases[APIProtocolAnthropic].(string); ok && strings.TrimSpace(value) != "" {
			selected = strings.TrimSpace(value)
		}
	}
	target, err := url.Parse(selected)
	if err != nil || target.User != nil || !strings.EqualFold(target.Scheme, configured.Scheme) || !strings.EqualFold(target.Host, configured.Host) {
		return "", errors.New("anthropic fallback must use the configured base_url origin")
	}
	return normalizeMixedBaseURL(target.String()), nil
}
