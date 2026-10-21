package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const MediaBillingKindGrokRefusal = "grok_refusal"
const grokRefusalRequestKey = "grok_refusal_request"

// GrokRefusalRepository commits the block and frozen charge atomically. Its
// implementation shares the usage ledger and the media billing recovery worker.
type GrokRefusalRepository interface {
	FindGrokRefusal(context.Context, int64, []string) (bool, error)
	CommitGrokRefusal(context.Context, *MediaBillingJob, []string, []string) (*MediaBillingJob, bool, error)
}

func GrokModerationRefusalBilledCost(group *Group) float64 {
	if group != nil && group.GrokModerationRefusalPrice != nil {
		return *group.GrokModerationRefusalPrice
	}
	return 0.1
}

func GrokModerationRefusalMessage() string {
	return "I'm sorry, I can't help with that request. Failed check: SAFETY_CHECK_TYPE"
}

type grokRefusalRequest struct {
	apiKey        *APIKey
	lookup, write []string
	input         OpenAIRecordUsageInput
	model         string
	stream        bool
	webSocket     bool
}

// CheckGrokRefusalRequest runs before admission or account selection and freezes
// only hashed conversation identity. A storage error must stop the request.
func (s *OpenAIGatewayService) CheckGrokRefusalRequest(c *gin.Context, key *APIKey, body []byte, updater APIKeyQuotaUpdater) (bool, error) {
	if s == nil || key == nil || key.User == nil {
		return false, errors.New("grok refusal gateway is unavailable")
	}
	repo, ok := s.usageBillingRepo.(GrokRefusalRepository)
	if !ok || s.mediaBillingJobs == nil {
		return false, errors.New("grok refusal repository is unavailable")
	}
	lookup, write := grokRefusalIdentity(c, body)
	request := &grokRefusalRequest{apiKey: key, lookup: lookup, write: write,
		model: gjson.GetBytes(body, "model").String(), stream: gjson.GetBytes(body, "stream").Bool(),
		input: OpenAIRecordUsageInput{APIKey: key, User: key.User, APIKeyService: updater,
			InboundEndpoint: c.Request.URL.Path, UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP(),
			SessionID: ExtractClientSessionID(c), RequestPayloadHash: HashUsageRequestPayload(body), QuotaPlatform: PlatformGrok}}
	c.Set(grokRefusalRequestKey, request)
	return repo.FindGrokRefusal(c.Request.Context(), key.User.ID, lookup)
}

// WebSocket turns bind the original client frame before admission or rewriting.
// The effective model may be inherited from an earlier response.create frame.
func (s *OpenAIGatewayService) CheckGrokRefusalWebSocketRequest(c *gin.Context, key *APIKey, body []byte, model string, updater APIKeyQuotaUpdater) (bool, error) {
	blocked, err := s.CheckGrokRefusalRequest(c, key, body, updater)
	if err != nil {
		return false, err
	}
	value, exists := c.Get(grokRefusalRequestKey)
	request, valid := value.(*grokRefusalRequest)
	if !exists || !valid || request == nil {
		return false, errors.New("grok refusal websocket request context is unavailable")
	}
	request.model = model
	request.stream = true
	request.webSocket = true
	return blocked, nil
}

type grokRefusalWSFailure struct{ error }

// IsGrokRefusalWSFailure excludes an atomically handled refusal (including a
// failed commit) from ordinary or standalone cyber-policy usage accounting.
func IsGrokRefusalWSFailure(err error) bool {
	var refusal *grokRefusalWSFailure
	return errors.As(err, &refusal)
}

func (e *grokRefusalWSFailure) Unwrap() error { return e.error }

func (s *OpenAIGatewayService) settleGrokRefusalWebSocketFailure(c *gin.Context, account *Account, forwardErr error) error {
	var upstream *openAIWSHTTPBridgeUpstreamError
	if account == nil || account.Platform != PlatformGrok || !errors.As(forwardErr, &upstream) || !isGrokBillableRefusal(upstream.StatusCode, upstream.ResponseBody) {
		return nil
	}
	value, exists := c.Get(grokRefusalRequestKey)
	request, valid := value.(*grokRefusalRequest)
	var err error
	if !exists || !valid || request == nil || !request.webSocket {
		err = errors.New("grok refusal websocket request context is unavailable")
	} else {
		err = s.settleGrokRefusal(c, account)
	}
	if err != nil {
		return &grokRefusalWSFailure{NewOpenAIWSClientCloseError(coderws.StatusInternalError, "Unable to persist upstream refusal; retry later", err)}
	}
	return &grokRefusalWSFailure{NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, GrokModerationRefusalMessage(), forwardErr)}
}

// settleGrokRefusal executes before writing HTTP 403. The transaction also fixes
// preauthorization finalization, so a crash cannot refund a committed charge.
// Once committed, recovery owns the event even if inline settlement fails.
func (s *OpenAIGatewayService) settleGrokRefusal(c *gin.Context, account *Account) error {
	value, bound := c.Get(grokRefusalRequestKey)
	if !bound {
		return nil
	} // non-text callers retain their existing error contract
	request, ok := value.(*grokRefusalRequest)
	if !ok || request == nil {
		return errors.New("grok refusal request context is invalid")
	}
	repo, ok := s.usageBillingRepo.(GrokRefusalRepository)
	if !ok {
		return errors.New("grok refusal repository is unavailable")
	}
	price := GrokModerationRefusalBilledCost(request.apiKey.Group)
	if err := validateGrokRefusalPrice(&price); err != nil {
		return err
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		price = 0
	}
	input := request.input
	input.Account = account
	input.Result = &OpenAIForwardResult{Model: request.model, UpstreamModel: request.model, Stream: request.stream, OpenAIWSMode: request.webSocket, GrokModerationRefusalPrice: &price}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 15*time.Second)
	defer cancel()
	intent, err := s.prepareMediaUsage(ctx, &input)
	if err != nil {
		return err
	}
	if intent.Command.APIKeyQuotaCost > 0 && input.APIKey.Key != "" {
		sum := sha256.Sum256([]byte(input.APIKey.Key))
		intent.AuthCacheKey = hex.EncodeToString(sum[:])
	}
	job := &MediaBillingJob{ID: "grok_refusal_" + HashUsageRequestPayload([]byte(fmt.Sprintf("%d:%s", intent.Command.APIKeyID, intent.Command.RequestID))),
		Kind: MediaBillingKindGrokRefusal, UserID: intent.Command.UserID, APIKeyID: intent.Command.APIKeyID,
		AccountID: intent.Command.AccountID, GroupID: derefGroupID(intent.Usage.GroupID), Status: MediaBillingReady, Intent: intent, AvailableAt: time.Now()}
	job, created, err := repo.CommitGrokRefusal(ctx, job, request.lookup, request.write)
	if err != nil {
		return err
	}
	if !created {
		return nil
	} // Current request's guard still belongs to its handler.
	var owned *BalancePreauthorizationGuard
	if guard, exists := BalancePreauthorizationGuardFromContext(ctx); exists {
		owned, _ = guard.TransferToWorker()
	}
	if job.Intent.Command.APIKeyQuotaCost > 0 {
		if invalidator, ok := input.APIKeyService.(apiKeyAuthCacheInvalidator); ok {
			invalidator.InvalidateAuthCacheByKey(ctx, input.APIKey.Key)
		}
	}
	if err := s.settleMediaBillingJob(ctx, job, owned); err != nil {
		logger.L().Warn("grok_refusal.settlement_pending", zap.String("job_id", job.ID), zap.Error(err))
	}
	return nil
}

func grokRefusalHash(namespace, value string) string {
	sum := sha256.Sum256([]byte("grok-refusal-v1:" + namespace + ":" + value))
	return hex.EncodeToString(sum[:])
}

// Strong IDs have separate namespaces. prompt_cache_key and previous_response_id
// are routing hints, not stable conversation IDs. Always keep the full refused
// transcript too, so removing an explicit ID cannot evade an unchanged history.
func grokRefusalIdentity(c *gin.Context, body []byte) (lookup, write []string) {
	seen := map[string]bool{}
	addID := func(namespace, raw string) {
		if id := sanitizeSessionID(raw); id != "" {
			key := grokRefusalHash(namespace, id)
			if !seen[key] {
				seen[key] = true
				lookup = append(lookup, key)
				write = append(write, key)
			}
		}
	}
	if c != nil && c.Request != nil {
		for _, header := range []string{"session-id", "session_id", "X-Session-Id", openCodeSessionAffinityHeader, openCodeNativeSessionHeader} {
			addID("session", c.GetHeader(header))
		}
		for _, header := range []string{"conversation_id", "X-Conversation-Id", grokConversationIDHeader} {
			addID("conversation", c.GetHeader(header))
		}
		addID("claude", ClaudeCodeSessionIDFromHeader(c))
	}
	root := openAIRequestPayloadView(body)
	for _, field := range []string{"session_id", "metadata.session_id"} {
		addID("session", root.Get(field).String())
	}
	for _, field := range []string{"conversation_id", "metadata.conversation_id"} {
		addID("conversation", root.Get(field).String())
	}
	if parsed := ParseMetadataUserID(root.Get("metadata.user_id").String()); parsed != nil {
		addID("claude", parsed.SessionID)
	}
	// Canonical JSON gives unambiguous framing and preserves exact text. Unknown
	// blocks/items retain their full structure, preventing unsafe aliasing.
	h := sha256.New()
	_, _ = h.Write([]byte("grok-refusal-transcript-v1"))
	var last string
	hasInput := false
	appendItem := func(item any, input bool) {
		raw, _ := json.Marshal(item)
		_, _ = fmt.Fprintf(h, "%d:", len(raw))
		_, _ = h.Write(raw)
		hasInput = hasInput || input
		if hasInput {
			last = hex.EncodeToString(h.Sum(nil))
			lookup = append(lookup, last)
		}
	}
	normalizeContent := func(content gjson.Result) any {
		if content.Type == gjson.String {
			return content.String()
		}
		if content.IsArray() {
			var text strings.Builder
			plain := true
			for _, block := range content.Array() {
				typ := block.Get("type").String()
				if typ != "text" && typ != "input_text" && typ != "output_text" {
					plain = false
					break
				}
				// Additional fields (e.g. images/annotations) must not disappear.
				fields := block.Map()
				if len(fields) != 2 || block.Get("text").Type != gjson.String {
					plain = false
					break
				}
				_, _ = text.WriteString(block.Get("text").String())
			}
			if plain {
				return text.String()
			}
		}
		return content.Value()
	}
	for _, field := range []string{"system", "instructions"} {
		if value := root.Get(field); value.Exists() {
			appendItem(map[string]any{"role": "system", "content": normalizeContent(value)}, false)
		}
	}
	items := root.Get("messages")
	if !items.Exists() {
		items = root.Get("input")
	}
	if items.Type == gjson.String {
		appendItem(map[string]any{"role": "user", "content": items.String()}, true)
	} else {
		for _, item := range items.Array() {
			role := item.Get("role").String()
			if role != "" && item.Get("content").Exists() {
				value := item.Map()
				normalized := map[string]any{}
				for key, field := range value {
					if key != "type" || field.String() != "message" {
						normalized[key] = field.Value()
					}
				}
				normalized["content"] = normalizeContent(item.Get("content"))
				appendItem(normalized, role != "system" && role != "developer")
			} else {
				appendItem(item.Value(), true)
			}
		}
	}
	if last != "" {
		write = append(write, last)
	}
	// No common-prefix or pre-latest-user write: only the complete refused input.
	return lookup, write
}

// writeGrokRefusalError retains the established format-specific writer while
// delaying its response until durable accounting has committed.
func (s *OpenAIGatewayService) writeGrokRefusalError(c *gin.Context, account *Account, message string, write compatErrorWriter) error {
	if err := s.settleGrokRefusal(c, account); err != nil {
		logger.L().Error("grok_refusal.commit_failed", zap.Error(err))
		writeGrokRefusalProtocolError(c, http.StatusServiceUnavailable, "api_error", "Unable to persist upstream refusal; retry later", write)
		return fmt.Errorf("persist Grok refusal: %w", err)
	}
	MarkResponseCommitted(c)
	writeGrokRefusalProtocolError(c, http.StatusForbidden, "invalid_request_error", message, write)
	return fmt.Errorf("grok content policy rejection: %s", message)
}

// UpdateGrokRefusalRoute uses the final selected billing group, preserving
// multi-group key routing rather than freezing its initial primary group.
func UpdateGrokRefusalRoute(c *gin.Context, key *APIKey, fields ChannelUsageFields) {
	if value, ok := c.Get(grokRefusalRequestKey); ok {
		request, ok := value.(*grokRefusalRequest)
		if !ok || request == nil {
			return
		}
		request.apiKey = key
		request.input.APIKey = key
		request.input.User = key.User
		request.input.ChannelUsageFields = fields
	}
}

// cyber_policy alone is an existing network/security event, not evidence of a
// content refusal. Keep its legacy path unless a specific safety phrase exists.
func isGrokBillableRefusal(status int, body []byte) bool {
	if !isGrokContentPolicyRejection(status, body) {
		return false
	}
	if hit, _, _ := detectOpenAICyberPolicy(body); hit && !grokContentPolicyMessage(string(body)) {
		return false
	}
	return true
}

func writeGrokRefusalProtocolError(c *gin.Context, status int, typ, message string, write compatErrorWriter) {
	if StopOpenAICompactSSEKeepaliveCommitted(c) || (c.Writer.Written() && strings.Contains(c.Writer.Header().Get("Content-Type"), "text/event-stream")) {
		MarkOpsStreamError(c, typ, message, status)
		var frame string
		switch {
		case strings.HasSuffix(c.Request.URL.Path, "/messages"):
			frame = buildAnthropicStreamErrorSSE(typ, message)
		case strings.HasSuffix(c.Request.URL.Path, "/chat/completions"):
			frame = buildChatStreamErrorSSE(typ, message)
		default:
			payload, _ := json.Marshal(map[string]any{"error": map[string]any{"type": typ, "message": message}})
			frame = buildOpenAIResponseFailedSSE("", "", time.Now().Unix(), payload, message)
		}
		_, _ = fmt.Fprint(c.Writer, frame)
		c.Writer.Flush()
		return
	}
	write(c, status, typ, message)
}
