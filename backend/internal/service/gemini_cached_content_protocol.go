package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
	"github.com/tidwall/sjson"
)

const (
	geminiCachedContentDefaultTTL              = time.Hour
	geminiCachedContentDefaultMaxTTL           = 24 * time.Hour
	geminiCachedContentDisplayNameMaxRunes     = 128
	geminiCachedContentUpstreamTagPrefix       = "s2a-"
	geminiCachedContentResponseMaxBytes        = 4 << 20
	geminiCachedContentUpstreamTimeout         = 5 * time.Minute
	geminiCachedContentMutationUpstreamTimeout = 30 * time.Second
	geminiCachedContentMaxWholeSeconds         = int64(math.MaxInt64) / int64(time.Second)

	GeminiExplicitCacheUpstreamCredentialKey       = "explicit_cache_upstream"
	GeminiExplicitCacheUpstreamMaxTTLCredentialKey = "explicit_cache_upstream_max_ttl_seconds"
)

var (
	// Only pre-dispatch failures carry RequestNotSent. A transport failure must
	// never be treated as permission to repeat a potentially completed mutation.
	ErrGeminiCachedContentRequestNotSent   = errors.New("gemini cached content request was not sent")
	ErrGeminiCachedContentOutcomeUncertain = errors.New("gemini cached content upstream outcome is uncertain")

	geminiCachedContentPublicIDPattern = regexp.MustCompile(`^[a-z0-9]{1,64}$`)
	geminiCachedContentAIStudioName    = regexp.MustCompile(`^cachedContents/[A-Za-z0-9_-]{1,256}$`)
	geminiCachedContentVertexName      = regexp.MustCompile(`^projects/([A-Za-z0-9][A-Za-z0-9_.:-]{0,127})/locations/([a-z][a-z0-9]*(?:-[a-z0-9]+)*)/cachedContents/[A-Za-z0-9_-]{1,256}$`)
	geminiCachedContentProjectPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
	geminiCachedContentLocationPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	geminiCachedContentDurationPattern = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]{1,9}))?s$`)
)

// Capability does not register routes, authorize a request, or start workers.
func IsGeminiExplicitCacheAccountType(account *Account) bool {
	return account != nil && account.Platform == PlatformGemini &&
		(account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount)
}

func IsGeminiExplicitCacheAccount(account *Account) bool {
	if !IsGeminiExplicitCacheAccountType(account) {
		return false
	}
	if account.Type == AccountTypeServiceAccount {
		mode := strings.TrimSpace(account.GetCredential("auth_mode"))
		return !account.IsVertexAPIKey() && (mode == "" || mode == "service_account") &&
			strings.TrimSpace(account.GetCredential("base_url")) == ""
	}
	if strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return false
	}
	// An explicitly selected OpenAI protocol cannot inherit native caching just
	// because its model name resembles a Gemini model. A custom native cache
	// endpoint requires its own opt-in, including when api_protocol is absent.
	switch strings.TrimSpace(account.GetCredential("api_protocol")) {
	case APIProtocolAdaptive, APIProtocolChatCompletions, APIProtocolAnthropic, APIProtocolResponses:
		return false
	}
	if _, err := geminiCachedContentAccountTTLLimit(account); err != nil {
		return false
	}
	return isOfficialGeminiAPIBaseURL(account.GetCredential("base_url")) || account.GeminiExplicitCacheUpstreamEnabled()
}

func (a *Account) GeminiExplicitCacheUpstreamEnabled() bool {
	if a == nil || a.Platform != PlatformGemini || a.Type != AccountTypeAPIKey {
		return false
	}
	enabled, _ := a.Credentials[GeminiExplicitCacheUpstreamCredentialKey].(bool)
	return enabled
}

// Invalid configured limits disable the account; they never remove its cap.
// The getter returns zero for an invalid or absent limit for compatibility;
// admission and mutations additionally check the error-bearing helper.
func (a *Account) GeminiExplicitCacheUpstreamMaxTTL() time.Duration {
	limit, _ := geminiCachedContentAccountTTLLimit(a)
	return limit
}

func geminiCachedContentAccountTTLLimit(a *Account) (time.Duration, error) {
	if !a.GeminiExplicitCacheUpstreamEnabled() {
		return 0, nil
	}
	v, exists := a.Credentials[GeminiExplicitCacheUpstreamMaxTTLCredentialKey]
	if !exists {
		return 0, nil
	}
	var raw string
	switch n := v.(type) {
	case string:
		raw = strings.TrimSpace(n)
	case json.Number:
		raw = string(n)
	case int:
		raw = strconv.Itoa(n)
	case int64:
		raw = strconv.FormatInt(n, 10)
	case float64:
		raw = strconv.FormatFloat(n, 'f', -1, 64)
	default:
		return 0, errors.New("invalid explicit cache upstream TTL limit")
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 || seconds > geminiCachedContentMaxWholeSeconds {
		return 0, errors.New("invalid explicit cache upstream TTL limit")
	}
	return time.Duration(seconds) * time.Second, nil
}

func NewGeminiCachedContentPublicID() (string, error) {
	var b [25]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])), nil
}

func ParseGeminiCachedContentPublicID(name string) (string, bool) {
	id := strings.TrimPrefix(strings.TrimSpace(name), GeminiCachedContentNamePrefix)
	if !geminiCachedContentPublicIDPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

type GeminiCachedContentReference struct {
	Name  string
	Paths []string
}

func FindGeminiCachedContentReference(body []byte) (*GeminiCachedContentReference, error) {
	root, err := decodeGeminiCachedContentObject(body)
	if err != nil {
		return nil, err
	}
	if _, exists := root["cached_content"]; exists {
		return nil, errors.New("use cachedContent instead of cached_content")
	}
	if _, exists := root["generate_content_request"]; exists {
		return nil, errors.New("use generateContentRequest instead of generate_content_request")
	}
	var ref *GeminiCachedContentReference
	add := func(obj map[string]any, path string) error {
		if _, exists := obj["cached_content"]; exists {
			return errors.New("use cachedContent instead of cached_content")
		}
		value, exists := obj["cachedContent"]
		if !exists {
			return nil
		}
		name, ok := value.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return errors.New("cachedContent must be a nonempty string")
		}
		name = strings.TrimSpace(name)
		if ref == nil {
			ref = &GeminiCachedContentReference{Name: name}
		} else if ref.Name != name {
			return errors.New("conflicting cachedContent references")
		}
		ref.Paths = append(ref.Paths, path)
		return nil
	}
	if err := add(root, "cachedContent"); err != nil {
		return nil, err
	}
	if value, exists := root["generateContentRequest"]; exists {
		nested, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("generateContentRequest must be an object")
		}
		if err := add(nested, "generateContentRequest.cachedContent"); err != nil {
			return nil, err
		}
	}
	return ref, nil
}

func RewriteGeminiCachedContentReference(body []byte, ref *GeminiCachedContentReference, upstreamName string) ([]byte, error) {
	if ref == nil {
		return body, nil
	}
	actual, err := FindGeminiCachedContentReference(body)
	if err != nil {
		return nil, err
	}
	if actual == nil || ref.Name != actual.Name || !slices.Equal(ref.Paths, actual.Paths) ||
		(!geminiCachedContentAIStudioName.MatchString(upstreamName) && !geminiCachedContentVertexName.MatchString(upstreamName)) {
		return nil, errors.New("invalid cachedContent rewrite")
	}
	out := body
	for _, path := range actual.Paths {
		out, err = sjson.SetBytes(out, path, upstreamName)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type GeminiCachedContentCreateRequest struct {
	Model       string
	TTL         time.Duration
	ExpireTime  time.Time // Preserve an absolute deadline; never restart it at dispatch.
	DisplayName string
	ttlExplicit bool
	payload     map[string]any
}

func (r *GeminiCachedContentCreateRequest) TTLSeconds() int64 {
	if r == nil || r.TTL <= 0 {
		return 0
	}
	ttl, err := geminiCachedContentWholeTTL(r.TTL)
	if err != nil {
		return 0
	}
	return int64(ttl / time.Second)
}

func (r *GeminiCachedContentCreateRequest) TTLForAccount(account *Account) (time.Duration, bool) {
	if r == nil || !IsGeminiExplicitCacheAccount(account) {
		return 0, false
	}
	ttl, err := geminiCachedContentWholeTTL(r.TTL)
	if err != nil {
		return 0, false
	}
	limit, err := geminiCachedContentAccountTTLLimit(account)
	if err != nil {
		return 0, false
	}
	if limit > 0 && ttl > limit {
		if r.ttlExplicit {
			return 0, false
		}
		ttl = limit
	}
	return ttl, true
}

func ParseGeminiCachedContentCreateRequest(body []byte, now time.Time, maxTTL time.Duration) (*GeminiCachedContentCreateRequest, error) {
	payload, err := decodeGeminiCachedContentPayload(body)
	if err != nil {
		return nil, err
	}
	rawModel, ok := payload["model"].(string)
	model := strings.TrimPrefix(strings.TrimSpace(rawModel), "models/")
	if !ok || !isSafeUpstreamPathSegment(model) {
		return nil, errors.New("model must be a model ID or models/{model_name}")
	}
	limit, err := geminiCachedContentMaxTTL(maxTTL)
	if err != nil {
		return nil, err
	}
	ttl, err := parseGeminiCachedContentExpiration(payload, now)
	if err != nil {
		return nil, err
	}
	explicit := ttl != 0
	if ttl == 0 {
		ttl = min(geminiCachedContentDefaultTTL, limit)
	}
	if ttl > limit {
		return nil, errors.New("ttl exceeds the maximum allowed")
	}
	displayName := ""
	if raw, exists := payload["displayName"]; exists {
		var valid bool
		displayName, valid = raw.(string)
		if !valid || !utf8.ValidString(displayName) || utf8.RuneCountInString(displayName) > geminiCachedContentDisplayNameMaxRunes {
			return nil, errors.New("displayName must be a string of at most 128 characters")
		}
	}
	expireTime := geminiCachedContentParsedAbsoluteExpiration(payload)
	for _, key := range []string{"name", "model", "displayName", "ttl", "expireTime", "createTime", "updateTime", "usageMetadata"} {
		delete(payload, key)
	}
	return &GeminiCachedContentCreateRequest{Model: model, TTL: ttl, ExpireTime: expireTime, DisplayName: displayName, ttlExplicit: explicit, payload: payload}, nil
}

type GeminiCachedContentPatchRequest struct {
	TTL        time.Duration
	ExpireTime time.Time
	Name       string
}

func ParseGeminiCachedContentPatchRequest(body []byte, updateMask string, now time.Time, maxTTL time.Duration) (*GeminiCachedContentPatchRequest, error) {
	payload, err := decodeGeminiCachedContentPayload(body)
	if err != nil {
		return nil, err
	}
	for key, value := range payload {
		switch key {
		case "ttl", "expireTime":
		case "name":
			name, ok := value.(string)
			if _, valid := ParseGeminiCachedContentPublicID(name); !ok || !valid {
				return nil, errors.New("invalid cached content name")
			}
		default:
			return nil, errors.New("only ttl or expireTime can be updated")
		}
	}
	if updateMask != "" {
		fields := strings.Split(updateMask, ",")
		if len(fields) != 1 {
			return nil, errors.New("updateMask must select one expiration field")
		}
		field := strings.TrimSpace(fields[0])
		if field == "expire_time" {
			field = "expireTime"
		}
		if (field != "ttl" && field != "expireTime") || payload[field] == nil {
			return nil, errors.New("updateMask must match the expiration field")
		}
	}
	ttl, err := parseGeminiCachedContentExpiration(payload, now)
	if err != nil {
		return nil, err
	}
	limit, err := geminiCachedContentMaxTTL(maxTTL)
	if err != nil {
		return nil, err
	}
	if ttl <= 0 || ttl > limit {
		return nil, errors.New("ttl or expireTime is required and must not exceed the maximum allowed")
	}
	name, _ := payload["name"].(string)
	return &GeminiCachedContentPatchRequest{TTL: ttl, ExpireTime: geminiCachedContentParsedAbsoluteExpiration(payload), Name: name}, nil
}

// Called only after expiration validation, retaining the original absolute
// timestamp instead of treating its duration at parse time as a fresh TTL.
func geminiCachedContentParsedAbsoluteExpiration(payload map[string]any) time.Time {
	raw, _ := payload["expireTime"].(string)
	expire, _ := time.Parse(time.RFC3339Nano, raw)
	return expire
}

var geminiCachedContentFieldAliases = map[string]string{
	"display_name": "displayName", "expire_time": "expireTime", "create_time": "createTime",
	"update_time": "updateTime", "usage_metadata": "usageMetadata",
}

func decodeGeminiCachedContentPayload(body []byte) (map[string]any, error) {
	payload, err := decodeGeminiCachedContentObject(body)
	if err != nil {
		return nil, err
	}
	for alias, canonical := range geminiCachedContentFieldAliases {
		if value, exists := payload[alias]; exists {
			if _, dup := payload[canonical]; dup {
				return nil, fmt.Errorf("only one of %s or %s may be set", canonical, alias)
			}
			delete(payload, alias)
			payload[canonical] = value
		}
	}
	return payload, nil
}

// Decode every object exactly once, retaining numeric spelling. encoding/json's
// map decoder alone accepts duplicate keys (including escaped spellings), which
// would make validation and forwarding disagree about the selected value.
func decodeGeminiCachedContentObject(body []byte) (map[string]any, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("body must be valid UTF-8 JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	value, err := decodeGeminiCachedContentValue(dec, 0)
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("body must be a JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("body must contain exactly one JSON object")
	}
	return obj, nil
}

func decodeGeminiCachedContentValue(dec *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("JSON nesting is too deep")
	}
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		obj := make(map[string]any)
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("invalid JSON object key")
			}
			if _, exists := obj[name]; exists {
				return nil, errors.New("duplicate JSON object key")
			}
			obj[name], err = decodeGeminiCachedContentValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
		}
		if end, err := dec.Token(); err != nil || end != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}
		return obj, nil
	case json.Delim('['):
		arr := make([]any, 0)
		for dec.More() {
			v, err := decodeGeminiCachedContentValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if end, err := dec.Token(); err != nil || end != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}
		return arr, nil
	default:
		return token, nil
	}
}

func parseGeminiCachedContentExpiration(payload map[string]any, now time.Time) (time.Duration, error) {
	rawTTL, hasTTL := payload["ttl"]
	rawExpire, hasExpire := payload["expireTime"]
	if hasTTL && hasExpire {
		return 0, errors.New("only one of ttl or expireTime may be set")
	}
	if hasTTL {
		raw, ok := rawTTL.(string)
		if !ok {
			return 0, errors.New("ttl must be a duration string")
		}
		return parseGoogleDurationSeconds(raw)
	}
	if hasExpire {
		raw, ok := rawExpire.(string)
		if !ok {
			return 0, errors.New("expireTime must be an RFC 3339 timestamp")
		}
		expire, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || !expire.After(now) || expire.After(now.Add(time.Duration(geminiCachedContentMaxWholeSeconds)*time.Second)) {
			return 0, errors.New("expireTime must be a representable future timestamp")
		}
		return geminiCachedContentWholeTTL(expire.Sub(now))
	}
	return 0, nil
}

func parseGoogleDurationSeconds(raw string) (time.Duration, error) {
	m := geminiCachedContentDurationPattern.FindStringSubmatch(raw)
	if m == nil {
		return 0, errors.New("ttl must be a protobuf duration such as 3600s")
	}
	seconds, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || seconds > geminiCachedContentMaxWholeSeconds {
		return 0, errors.New("ttl is too large")
	}
	if strings.Trim(m[2], "0") != "" {
		seconds++
	}
	if seconds <= 0 || seconds > geminiCachedContentMaxWholeSeconds {
		return 0, errors.New("ttl must be positive and representable")
	}
	return time.Duration(seconds) * time.Second, nil
}

func geminiCachedContentWholeTTL(ttl time.Duration) (time.Duration, error) {
	if ttl <= 0 {
		return 0, errors.New("ttl must be positive")
	}
	seconds := int64(ttl / time.Second)
	if ttl%time.Second != 0 {
		seconds++
	}
	if seconds > geminiCachedContentMaxWholeSeconds {
		return 0, errors.New("ttl is too large")
	}
	return time.Duration(seconds) * time.Second, nil
}

func geminiCachedContentMaxTTL(limit time.Duration) (time.Duration, error) {
	if limit == 0 {
		limit = geminiCachedContentDefaultMaxTTL
	}
	if limit < time.Second {
		return 0, errors.New("maximum ttl must be at least one second")
	}
	return limit / time.Second * time.Second, nil
}

type GeminiCachedContentUpstream struct {
	EndpointIdentity string
	Name             string
	Model            string
	TotalTokenCount  int64
	ExpireTime       time.Time
}

type GeminiCachedContentUpstreamError struct {
	StatusCode      int
	Body            []byte
	UpstreamGateway bool
}

func (e *GeminiCachedContentUpstreamError) Error() string {
	return fmt.Sprintf("gemini cached content upstream error: %d", e.StatusCode)
}

func (e *GeminiCachedContentUpstreamError) NotFound() bool {
	if e == nil {
		return false
	}
	if isGeminiCachedContentNotFound(e.StatusCode, e.Body) {
		return true
	}
	if e.StatusCode != http.StatusNotFound {
		return false
	}
	payload, err := decodeGeminiCachedContentObject(e.Body)
	if err != nil {
		return false
	}
	upstream, ok := payload["error"].(map[string]any)
	if !ok || upstream["status"] != "NOT_FOUND" || upstream["code"] != json.Number("404") {
		return false
	}
	message, _ := upstream["message"].(string)
	message = strings.ToLower(message)
	// A relay's missing API route is not proof that the held resource is gone.
	for _, route := range []string{"endpoint", "method", "route", " api"} {
		if strings.Contains(message, route) {
			return false
		}
	}
	return strings.Contains(message, "cachedcontent not found") ||
		(strings.Contains(message, "cached content") && (strings.Contains(message, "not found") || strings.Contains(message, "does not exist")))
}

func (e *GeminiCachedContentUpstreamError) Is(target error) bool {
	return target == ErrGeminiCachedContentOutcomeUncertain && (e.StatusCode >= 500 || e.StatusCode == http.StatusRequestTimeout)
}

// Name is usable for later cleanup only after verifying it belongs to the
// selected account's resource collection. No untrusted response URL is used.
type GeminiCachedContentMalformedResponseError struct {
	Name string
	Err  error
}

func (e *GeminiCachedContentMalformedResponseError) Error() string {
	return "gemini cached content upstream returned an unusable response: " + e.Err.Error()
}

func (e *GeminiCachedContentMalformedResponseError) Unwrap() error { return e.Err }

func (e *GeminiCachedContentMalformedResponseError) Is(target error) bool {
	return target == ErrGeminiCachedContentOutcomeUncertain
}

type GeminiCachedContentUncertainOutcomeError struct{ Err error }

func (e *GeminiCachedContentUncertainOutcomeError) Error() string {
	return "gemini cached content upstream outcome is uncertain: " + e.Err.Error()
}

func (e *GeminiCachedContentUncertainOutcomeError) Unwrap() error { return e.Err }

func (e *GeminiCachedContentUncertainOutcomeError) Is(target error) bool {
	return target == ErrGeminiCachedContentOutcomeUncertain
}

// Protocol primitives only: no DI registration, HTTP routes, persistence,
// billing, account failover, automatic retries or recovery worker are enabled.
type GeminiCachedContentService struct {
	repo   GeminiCachedContentRepository
	compat *GeminiMessagesCompatService
	maxTTL time.Duration
	now    func() time.Time
}

func NewGeminiCachedContentService(repo GeminiCachedContentRepository, compat *GeminiMessagesCompatService, maxTTL time.Duration) *GeminiCachedContentService {
	if maxTTL == 0 {
		maxTTL = geminiCachedContentDefaultMaxTTL
	}
	return &GeminiCachedContentService{repo: repo, compat: compat, maxTTL: maxTTL, now: time.Now}
}

func (s *GeminiCachedContentService) Now() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *GeminiCachedContentService) MaxTTL() time.Duration { return s.maxTTL }

func (s *GeminiCachedContentService) MaxTTLForAccount(account *Account) time.Duration {
	if s == nil || !IsGeminiExplicitCacheAccount(account) {
		return 0
	}
	limit, err := geminiCachedContentMaxTTL(s.maxTTL)
	if err != nil {
		return 0
	}
	if upstream, err := geminiCachedContentAccountTTLLimit(account); err != nil {
		return 0
	} else if upstream > 0 {
		limit = min(limit, upstream)
	}
	return limit
}

func GeminiCachedContentUpstreamModelResource(account *Account, mappedModel string) (string, error) {
	if !IsGeminiExplicitCacheAccount(account) || !isSafeUpstreamPathSegment(mappedModel) {
		return "", errors.New("unsupported explicit cache account or model")
	}
	if account.Type == AccountTypeAPIKey {
		return "models/" + mappedModel, nil
	}
	project, location, err := geminiCachedContentVertexScope(account, mappedModel)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", project, location, mappedModel), nil
}

func (s *GeminiCachedContentService) validateTTL(account *Account, ttl time.Duration) (time.Duration, error) {
	whole, err := geminiCachedContentWholeTTL(ttl)
	if err != nil {
		return 0, err
	}
	limit := s.MaxTTLForAccount(account)
	if limit <= 0 || whole > limit {
		return 0, errors.New("ttl exceeds explicit cache account limit")
	}
	return whole, nil
}

func (s *GeminiCachedContentService) CreateUpstream(ctx context.Context, account *Account, req *GeminiCachedContentCreateRequest, mappedModel, publicID string, ttl time.Duration) (*GeminiCachedContentUpstream, error) {
	if s == nil || req == nil || req.payload == nil || !geminiCachedContentPublicIDPattern.MatchString(publicID) {
		return nil, geminiCachedContentNotSent(errors.New("invalid cache create request"))
	}
	model, err := GeminiCachedContentUpstreamModelResource(account, mappedModel)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	ttl, err = s.validateTTL(account, ttl)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	if ttl > req.TTL || (req.ttlExplicit && ttl != req.TTL) {
		return nil, geminiCachedContentNotSent(errors.New("upstream ttl differs from the validated create request"))
	}
	payload := make(map[string]any, len(req.payload)+3)
	for key, value := range req.payload {
		payload[key] = value
	}
	payload["model"], payload["ttl"], payload["displayName"] = model, fmt.Sprintf("%ds", ttl/time.Second), geminiCachedContentUpstreamTagPrefix+publicID
	if !req.ExpireTime.IsZero() {
		if _, err := s.validateTTL(account, req.ExpireTime.Sub(s.Now())); err != nil {
			return nil, geminiCachedContentNotSent(err)
		}
		delete(payload, "ttl")
		payload["expireTime"] = req.ExpireTime.UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	base, err := s.apiBase(account)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	target, err := geminiCachedContentCollectionURL(account, base, mappedModel)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	identity, err := GeminiCachedContentEndpointIdentity(account, mappedModel)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	response, err := s.doUpstream(ctx, account, http.MethodPost, target, body, req.ExpireTime, geminiCachedContentUpstreamTimeout)
	if err != nil {
		return nil, err
	}
	result, err := s.parseUpstream(response, account, mappedModel, model, "", ttl, req.ExpireTime, true)
	if err != nil {
		return nil, err
	}
	result.EndpointIdentity = identity
	return result, nil
}

func (s *GeminiCachedContentService) PatchUpstream(ctx context.Context, account *Account, record *GeminiCachedContent, request *GeminiCachedContentPatchRequest) (*GeminiCachedContentUpstream, error) {
	if request == nil || record == nil || (request.Name != "" && request.Name != record.PublicName() && request.Name != record.PublicID) {
		return nil, geminiCachedContentNotSent(errors.New("invalid cache patch request or resource name"))
	}
	target, err := s.upstreamResourceURL(account, record)
	if err != nil {
		return nil, err
	}
	ttl, err := s.validateTTL(account, request.TTL)
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	field, value := "ttl", fmt.Sprintf("%ds", ttl/time.Second)
	if !request.ExpireTime.IsZero() {
		if _, err := s.validateTTL(account, request.ExpireTime.Sub(s.Now())); err != nil {
			return nil, geminiCachedContentNotSent(err)
		}
		field, value = "expireTime", request.ExpireTime.UTC().Format(time.RFC3339Nano)
	}
	body, _ := json.Marshal(map[string]string{field: value})
	response, err := s.doUpstream(ctx, account, http.MethodPatch, target+"?updateMask="+field, body, request.ExpireTime, geminiCachedContentMutationUpstreamTimeout)
	if err != nil {
		return nil, err
	}
	result, err := s.parseUpstream(response, account, record.UpstreamModelID(), record.UpstreamModel, record.UpstreamName, ttl, request.ExpireTime, false)
	if err != nil {
		return nil, err
	}
	result.EndpointIdentity = record.UpstreamEndpointIdentity
	return result, nil
}

func (s *GeminiCachedContentService) DeleteUpstream(ctx context.Context, account *Account, record *GeminiCachedContent) error {
	target, err := s.upstreamResourceURL(account, record)
	if err != nil {
		return err
	}
	_, err = s.doUpstream(ctx, account, http.MethodDelete, target, nil, time.Time{}, geminiCachedContentMutationUpstreamTimeout)
	return err
}

func (s *GeminiCachedContentService) upstreamResourceURL(account *Account, record *GeminiCachedContent) (string, error) {
	if s == nil || record == nil || !record.MatchesEndpoint(account) {
		return "", geminiCachedContentNotSent(errors.New("cache holding account mismatch"))
	}
	model, err := GeminiCachedContentUpstreamModelResource(account, record.UpstreamModelID())
	if err != nil || model != record.UpstreamModel || !geminiCachedContentNameForAccount(account, record.UpstreamModelID(), record.UpstreamName) {
		return "", geminiCachedContentNotSent(errors.New("cache resource does not belong to the selected account scope"))
	}
	base, err := s.apiBase(account)
	if err != nil {
		return "", geminiCachedContentNotSent(err)
	}
	target, err := geminiCachedContentResourceURL(account, base, record.UpstreamName)
	if err != nil {
		return "", geminiCachedContentNotSent(err)
	}
	return target, nil
}

func geminiCachedContentNotSent(err error) error {
	return fmt.Errorf("%w: %w", ErrGeminiCachedContentRequestNotSent, err)
}

// Caller cancellation is honored before dispatch. Once a mutation is sent,
// finish reading its bounded response even if the client disconnects, so a
// future lifecycle can persist or recover its result. Mutations are sent once.
func (s *GeminiCachedContentService) doUpstream(ctx context.Context, account *Account, method, target string, body []byte, expiresAt time.Time, timeout time.Duration) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.compat == nil || s.compat.httpUpstream == nil || !IsGeminiExplicitCacheAccount(account) || timeout <= 0 {
		return nil, geminiCachedContentNotSent(errors.New("explicit cache upstream is not configured or supported"))
	}
	if method == http.MethodPost {
		timeout = min(timeout, geminiCachedContentUpstreamTimeout)
	} else {
		timeout = min(timeout, geminiCachedContentMutationUpstreamTimeout)
	}
	if err := ctx.Err(); err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	if err := s.validateTarget(account, method, target); err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	deadline := time.Now().Add(timeout)
	authCtx, authCancel := context.WithDeadline(ctx, deadline)
	defer authCancel()
	req, err := http.NewRequestWithContext(authCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if account.Type == AccountTypeAPIKey {
		req.Header.Set("x-goog-api-key", strings.TrimSpace(account.GetCredential("api_key")))
	} else if err := setVertexGeminiAuth(authCtx, req, account, s.compat.tokenProvider); err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	if err := authCtx.Err(); err != nil {
		return nil, geminiCachedContentNotSent(err)
	}
	if !expiresAt.IsZero() && !expiresAt.After(s.Now()) {
		return nil, geminiCachedContentNotSent(errors.New("absolute cache expiration passed before dispatch"))
	}
	// Reads remain cancelable throughout authentication, dispatch and body reads.
	// Only mutations need to finish after their caller disconnects.
	sendParent := ctx
	if method != http.MethodGet {
		sendParent = context.WithoutCancel(ctx)
	}
	sendCtx, sendCancel := context.WithDeadline(WithHTTPUpstreamRedirectsDisabled(sendParent), deadline)
	defer sendCancel()
	req = req.WithContext(sendCtx)
	resp, err := s.compat.httpUpstream.Do(req, vertexServiceAccountProxyURL(account), account.ID, account.Concurrency)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, &GeminiCachedContentUncertainOutcomeError{Err: err}
	}
	if resp == nil || resp.Body == nil {
		return nil, &GeminiCachedContentMalformedResponseError{Err: errors.New("empty upstream response")}
	}
	defer func() { _ = resp.Body.Close() }()
	response, err := io.ReadAll(io.LimitReader(resp.Body, geminiCachedContentResponseMaxBytes+1))
	if err != nil {
		return nil, &GeminiCachedContentUncertainOutcomeError{Err: err}
	}
	if len(response) > geminiCachedContentResponseMaxBytes {
		return nil, &GeminiCachedContentMalformedResponseError{Err: errors.New("upstream response exceeds size limit")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &GeminiCachedContentUpstreamError{StatusCode: resp.StatusCode, Body: response, UpstreamGateway: account.GeminiExplicitCacheUpstreamEnabled()}
	}
	return response, nil
}

func (s *GeminiCachedContentService) apiBase(account *Account) (string, error) {
	if !IsGeminiExplicitCacheAccount(account) || s == nil || s.compat == nil || s.compat.cfg == nil {
		return "", errors.New("explicit cache account or URL policy is not configured")
	}
	if account.Type == AccountTypeServiceAccount {
		return "", nil
	}
	raw := account.GetCredential("base_url")
	if isOfficialGeminiAPIBaseURL(raw) {
		raw = geminicli.AIStudioBaseURL
	}
	base, err := geminiCachedContentCleanBase(raw)
	if err != nil {
		return "", err
	}
	return s.compat.validateUpstreamBaseURL(base)
}

func geminiCachedContentCleanBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Opaque != "" ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return "", errors.New("invalid explicit cache base URL")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if _, valid := sanitizedUpstreamPathSuffix(u.Path); !valid {
		return "", errors.New("invalid explicit cache base URL path")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// Defense at the credential attachment boundary: even an internal caller may
// only address this account's cache API, never an arbitrary URL or redirect.
func (s *GeminiCachedContentService) validateTarget(account *Account, method, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.User != nil || u.RawPath != "" || u.Fragment != "" || u.ForceQuery {
		return errors.New("invalid explicit cache target")
	}
	if (method == http.MethodPatch && u.RawQuery != "updateMask=ttl" && u.RawQuery != "updateMask=expireTime") ||
		(method != http.MethodPatch && method != http.MethodGet && u.RawQuery != "") ||
		(method == http.MethodGet && u.RawQuery != "" && !geminiCachedContentValidListQuery(u.RawQuery)) {
		return errors.New("invalid explicit cache target query")
	}
	queryless := *u
	queryless.RawQuery = ""
	var resource string
	if account.Type == AccountTypeAPIKey {
		base, err := s.apiBase(account)
		if err != nil {
			return err
		}
		prefix := base + "/v1beta/"
		if !strings.HasPrefix(queryless.String(), prefix) {
			return errors.New("explicit cache target origin mismatch")
		}
		resource = strings.TrimPrefix(queryless.String(), prefix)
	} else {
		resource = strings.TrimPrefix(u.Path, "/v1/")
		parts := strings.Split(resource, "/")
		segments := 6
		if method == http.MethodPost || (method == http.MethodGet && u.RawQuery != "") {
			segments = 5
		}
		if len(parts) != segments || parts[0] != "projects" || parts[1] != account.VertexProjectID() ||
			parts[2] != "locations" || !geminiCachedContentAllowedVertexLocation(account, parts[3]) ||
			parts[4] != "cachedContents" || u.Scheme != "https" || u.Host != geminiCachedContentVertexHost(parts[3]) {
			return errors.New("explicit cache target Vertex scope mismatch")
		}
		if s.compat.cfg == nil {
			return errors.New("explicit cache URL policy is not configured")
		}
		if _, err := s.compat.validateUpstreamBaseURL("https://" + u.Host); err != nil {
			return err
		}
		if method == http.MethodPost || (method == http.MethodGet && u.RawQuery != "") {
			resource = "cachedContents"
		}
	}
	if method == http.MethodPost && resource == "cachedContents" {
		return nil
	}
	if method == http.MethodGet && resource == "cachedContents" && geminiCachedContentValidListQuery(u.RawQuery) {
		return nil
	}
	if (method == http.MethodPatch || method == http.MethodDelete || (method == http.MethodGet && u.RawQuery == "")) &&
		(geminiCachedContentAIStudioName.MatchString(resource) || (account.Type == AccountTypeServiceAccount && geminiCachedContentVertexName.MatchString(resource))) {
		return nil
	}
	return errors.New("invalid explicit cache method or resource")
}

func geminiCachedContentVertexScope(account *Account, mappedModel string) (string, string, error) {
	project, location := account.VertexProjectID(), account.VertexLocation(mappedModel)
	if !geminiCachedContentProjectPattern.MatchString(project) || strings.Contains(project, "..") ||
		len(location) > 63 || !geminiCachedContentLocationPattern.MatchString(location) {
		return "", "", errors.New("invalid explicit cache Vertex project or location")
	}
	return project, location, nil
}

func geminiCachedContentAllowedVertexLocation(account *Account, location string) bool {
	if len(location) > 63 || !geminiCachedContentLocationPattern.MatchString(location) {
		return false
	}
	if location == account.VertexLocation("") {
		return true
	}
	if locations, ok := account.Credentials["vertex_model_locations"].(map[string]any); ok {
		for model, configured := range locations {
			if value, ok := configured.(string); ok && isSafeUpstreamPathSegment(model) && strings.TrimSpace(value) == location {
				return true
			}
		}
	}
	return false
}

func geminiCachedContentNameForAccount(account *Account, mappedModel, name string) bool {
	if !IsGeminiExplicitCacheAccount(account) {
		return false
	}
	if account.Type == AccountTypeAPIKey {
		return geminiCachedContentAIStudioName.MatchString(name)
	}
	project, location, err := geminiCachedContentVertexScope(account, mappedModel)
	parts := geminiCachedContentVertexName.FindStringSubmatch(name)
	return err == nil && parts != nil && parts[1] == project && parts[2] == location
}

func geminiCachedContentCollectionURL(account *Account, base, mappedModel string) (string, error) {
	if !IsGeminiExplicitCacheAccount(account) || !isSafeUpstreamPathSegment(mappedModel) {
		return "", errors.New("unsupported explicit cache account or model")
	}
	if account.Type == AccountTypeAPIKey {
		clean, err := geminiCachedContentCleanBase(base)
		if err != nil {
			return "", err
		}
		return clean + "/v1beta/cachedContents", nil
	}
	project, location, err := geminiCachedContentVertexScope(account, mappedModel)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/cachedContents", geminiCachedContentVertexHost(location), project, location), nil
}

func geminiCachedContentResourceURL(account *Account, base, name string) (string, error) {
	if !IsGeminiExplicitCacheAccount(account) {
		return "", errors.New("unsupported explicit cache account")
	}
	if account.Type == AccountTypeAPIKey {
		if !geminiCachedContentAIStudioName.MatchString(name) {
			return "", errors.New("invalid AI Studio cache resource name")
		}
		clean, err := geminiCachedContentCleanBase(base)
		if err != nil {
			return "", err
		}
		return clean + "/v1beta/" + name, nil
	}
	parts := geminiCachedContentVertexName.FindStringSubmatch(name)
	if parts == nil || parts[1] != account.VertexProjectID() || !geminiCachedContentAllowedVertexLocation(account, parts[2]) {
		return "", errors.New("invalid Vertex cache resource scope")
	}
	return "https://" + geminiCachedContentVertexHost(parts[2]) + "/v1/" + name, nil
}

func geminiCachedContentVertexHost(location string) string {
	if location == "global" {
		return vertexGlobalHost
	}
	return location + "-aiplatform.googleapis.com"
}

func (s *GeminiCachedContentService) parseUpstream(body []byte, account *Account, mappedModel, expectedModel, expectedName string, ttl time.Duration, absoluteExpiry time.Time, requireTokens bool) (*GeminiCachedContentUpstream, error) {
	payload, err := decodeGeminiCachedContentObject(body)
	if err != nil {
		return nil, &GeminiCachedContentMalformedResponseError{Err: err}
	}
	name, _ := payload["name"].(string)
	if !geminiCachedContentNameForAccount(account, mappedModel, name) || (expectedName != "" && name != expectedName) {
		return nil, &GeminiCachedContentMalformedResponseError{Err: errors.New("unexpected cache resource name or account scope")}
	}
	fail := func(reason string) (*GeminiCachedContentUpstream, error) {
		return nil, &GeminiCachedContentMalformedResponseError{Name: name, Err: errors.New(reason)}
	}
	model, _ := payload["model"].(string)
	if model != expectedModel {
		return fail("unexpected cache model")
	}
	rawExpire, _ := payload["expireTime"].(string)
	expire, err := time.Parse(time.RFC3339Nano, rawExpire)
	now := s.Now()
	latest := now.Add(ttl)
	if !absoluteExpiry.IsZero() {
		latest = absoluteExpiry
	}
	if err != nil || !expire.After(now) || expire.After(latest) {
		return fail("missing, expired or out-of-bounds cache expiration")
	}
	out := &GeminiCachedContentUpstream{Name: name, Model: model, ExpireTime: expire}
	if usage, exists := payload["usageMetadata"]; exists {
		metadata, ok := usage.(map[string]any)
		if !ok {
			return fail("invalid cache usage metadata")
		}
		if raw, exists := metadata["totalTokenCount"]; exists {
			number, ok := raw.(json.Number)
			if !ok {
				return fail("invalid cache token count")
			}
			out.TotalTokenCount, err = number.Int64()
			if err != nil || out.TotalTokenCount <= 0 {
				return fail("invalid cache token count")
			}
		}
	}
	if requireTokens && out.TotalTokenCount <= 0 {
		return fail("missing usageMetadata.totalTokenCount")
	}
	return out, nil
}
