package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	geminiCachedContentRecoveryReadTimeout = 30 * time.Second
	geminiCachedContentRecoveryPageSize    = 100
	geminiCachedContentRecoveryMaxPages    = 10
	geminiCachedContentRecoveryMaxToken    = 2048
)

// GeminiCachedContentRecoveryReadError deliberately does not unwrap the read's
// error. Even a pre-dispatch read failure says nothing about whether the original
// create was dispatched: callers must never classify it as RequestNotSent, use
// absence to refund a create, or retry its POST. Reasons contain no upstream body.
type GeminiCachedContentRecoveryReadError struct{ Reason string }

func (e *GeminiCachedContentRecoveryReadError) Error() string {
	return "gemini cached content recovery remains uncertain: " + e.Reason
}

func (e *GeminiCachedContentRecoveryReadError) Is(target error) bool {
	return target == ErrGeminiCachedContentOutcomeUncertain
}

func geminiCachedContentRecoveryUncertain(reason string) error {
	return &GeminiCachedContentRecoveryReadError{Reason: reason}
}

func geminiCachedContentRecoveryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	// One shared budget includes every page, credential lookup and confirmation.
	return context.WithTimeout(ctx, geminiCachedContentRecoveryReadTimeout)
}

func (s *GeminiCachedContentService) validateRecoveryBinding(account *Account, record *GeminiCachedContent) error {
	// MatchesEndpoint is local, including service-account principal parsing. It
	// must run before API policy/authentication can cause any network activity.
	if s == nil || record == nil || !record.MatchesEndpoint(account) ||
		!geminiCachedContentPublicIDPattern.MatchString(record.PublicID) ||
		record.TotalTokenCount < 0 || !record.ExpireTime.After(s.Now()) {
		return geminiCachedContentRecoveryUncertain("invalid or changed frozen binding")
	}
	model, err := GeminiCachedContentUpstreamModelResource(account, record.UpstreamModelID())
	if err != nil || model != record.UpstreamModel {
		return geminiCachedContentRecoveryUncertain("frozen model does not match account scope")
	}
	return nil
}

// GetUpstream reads only the exact frozen binding and returns validated metadata.
// It neither persists a result nor changes lifecycle, billing or retry state.
// Missing/expired resources and every read failure leave the original mutation
// unresolved; no arbitrary upstream response or cached contents escape here.
func (s *GeminiCachedContentService) GetUpstream(ctx context.Context, account *Account, record *GeminiCachedContent) (*GeminiCachedContentUpstream, error) {
	ctx, cancel := geminiCachedContentRecoveryContext(ctx)
	defer cancel()
	if err := s.validateRecoveryBinding(account, record); err != nil {
		return nil, err
	}
	target, err := s.upstreamResourceURL(account, record)
	if err != nil {
		return nil, geminiCachedContentRecoveryUncertain("frozen resource is not readable")
	}
	body, err := s.doUpstream(ctx, account, http.MethodGet, target, nil, record.ExpireTime, geminiCachedContentRecoveryReadTimeout)
	if err != nil || ctx.Err() != nil {
		return nil, geminiCachedContentRecoveryUncertain("resource read failed")
	}
	payload, err := decodeGeminiCachedContentObject(body)
	if err != nil {
		return nil, geminiCachedContentRecoveryUncertain("invalid resource JSON")
	}
	out, err := s.parseRecoveryMetadata(payload, account, record, record.UpstreamName)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !out.ExpireTime.After(s.Now()) {
		return nil, geminiCachedContentRecoveryUncertain("resource read expired")
	}
	return out, nil
}

// DiscoverCreatedUpstream resolves a response-lost create only after a complete,
// bounded native list scan finds exactly one exact s2a-PublicID tag and GET
// confirms its name, model, token count and expiration. The pending binding must
// already freeze AccountID, UpstreamModel, endpoint identity and expiration upper
// bound, with no UpstreamName. Opt-out, credential/destination changes, absence,
// duplicate tags, malformed pages and scan limits all remain uncertain. This is
// a read primitive, not a recovery worker, POST retry or absence-based refund.
func (s *GeminiCachedContentService) DiscoverCreatedUpstream(ctx context.Context, account *Account, pending *GeminiCachedContent) (*GeminiCachedContentUpstream, error) {
	ctx, cancel := geminiCachedContentRecoveryContext(ctx)
	defer cancel()
	if err := s.validateRecoveryBinding(account, pending); err != nil {
		return nil, err
	}
	if pending.UpstreamName != "" {
		return nil, geminiCachedContentRecoveryUncertain("discovery requires a pending create")
	}
	base, err := s.apiBase(account)
	if err != nil {
		return nil, geminiCachedContentRecoveryUncertain("frozen collection is not readable")
	}
	collection, err := geminiCachedContentCollectionURL(account, base, pending.UpstreamModelID())
	if err != nil {
		return nil, geminiCachedContentRecoveryUncertain("invalid frozen collection")
	}
	var match *GeminiCachedContentUpstream
	seen := make(map[string]bool)
	pageToken := ""
	for page := 0; page < geminiCachedContentRecoveryMaxPages; page++ {
		if err := s.validateRecoveryBinding(account, pending); err != nil {
			return nil, err
		}
		query := url.Values{"pageSize": {"100"}}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		body, err := s.doUpstream(ctx, account, http.MethodGet, collection+"?"+query.Encode(), nil, pending.ExpireTime, geminiCachedContentRecoveryReadTimeout)
		if err != nil || ctx.Err() != nil {
			return nil, geminiCachedContentRecoveryUncertain("collection read failed")
		}
		payload, err := decodeGeminiCachedContentObject(body)
		if err != nil {
			return nil, geminiCachedContentRecoveryUncertain("invalid collection JSON")
		}
		entries, ok := payload["cachedContents"].([]any)
		if !ok || len(entries) > geminiCachedContentRecoveryPageSize {
			return nil, geminiCachedContentRecoveryUncertain("invalid or oversized native collection")
		}
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok || !geminiCachedContentValidRecoveryListEntry(entry, account, pending) {
				return nil, geminiCachedContentRecoveryUncertain("invalid collection entry")
			}
			tag, hasTag := entry["displayName"]
			if hasTag {
				if _, ok := tag.(string); !ok {
					return nil, geminiCachedContentRecoveryUncertain("invalid collection tag")
				}
			}
			if tag != geminiCachedContentUpstreamTagPrefix+pending.PublicID {
				continue
			}
			candidate, err := s.parseRecoveryMetadata(entry, account, pending, "")
			if err != nil {
				return nil, err
			}
			if match != nil {
				return nil, geminiCachedContentRecoveryUncertain("multiple matching create tags")
			}
			match = candidate
		}
		next := ""
		if raw, exists := payload["nextPageToken"]; exists {
			var ok bool
			next, ok = raw.(string)
			if !ok || !geminiCachedContentValidPageToken(next) {
				return nil, geminiCachedContentRecoveryUncertain("invalid collection page token")
			}
		}
		if next == "" {
			if match == nil {
				return nil, geminiCachedContentRecoveryUncertain("no matching create tag")
			}
			confirmed := *pending
			confirmed.UpstreamName, confirmed.TotalTokenCount = match.Name, match.TotalTokenCount
			// The shared parent deadline prevents this GET from restarting the budget.
			out, err := s.GetUpstream(ctx, account, &confirmed)
			if err != nil {
				return nil, err
			}
			if !out.ExpireTime.Equal(match.ExpireTime) {
				return nil, geminiCachedContentRecoveryUncertain("resource changed during confirmation")
			}
			return out, nil
		}
		if seen[next] {
			return nil, geminiCachedContentRecoveryUncertain("collection page token cycle")
		}
		seen[next], pageToken = true, next
	}
	return nil, geminiCachedContentRecoveryUncertain("collection scan limit reached")
}

func (s *GeminiCachedContentService) parseRecoveryMetadata(payload map[string]any, account *Account, record *GeminiCachedContent, expectedName string) (*GeminiCachedContentUpstream, error) {
	if payload["displayName"] != geminiCachedContentUpstreamTagPrefix+record.PublicID {
		return nil, geminiCachedContentRecoveryUncertain("resource tag mismatch")
	}
	// Reuse the mutation parser's model, account scope, positive integer token
	// count and absolute expiration checks, returning only its typed metadata.
	body, err := json.Marshal(map[string]any{
		"name": payload["name"], "model": payload["model"],
		"expireTime": payload["expireTime"], "usageMetadata": payload["usageMetadata"],
	})
	if err != nil {
		return nil, geminiCachedContentRecoveryUncertain("invalid resource metadata")
	}
	out, err := s.parseUpstream(body, account, record.UpstreamModelID(), record.UpstreamModel, expectedName, 0, record.ExpireTime, true)
	if err != nil || (record.TotalTokenCount > 0 && out.TotalTokenCount != record.TotalTokenCount) {
		return nil, geminiCachedContentRecoveryUncertain("resource metadata mismatch")
	}
	out.EndpointIdentity = record.UpstreamEndpointIdentity
	return out, nil
}

// Even unrelated entries must be native metadata, not a malformed suffix hidden
// after an early match. Expired unrelated entries are allowed during pagination;
// the candidate itself must still satisfy parseUpstream's future/upper bounds.
func geminiCachedContentValidRecoveryListEntry(entry map[string]any, account *Account, pending *GeminiCachedContent) bool {
	name, ok := entry["name"].(string)
	if !ok || !geminiCachedContentNameForAccount(account, pending.UpstreamModelID(), name) {
		return false
	}
	model, ok := entry["model"].(string)
	if !ok {
		return false
	}
	modelID := model[strings.LastIndex(model, "/")+1:]
	prefix := strings.TrimSuffix(pending.UpstreamModel, pending.UpstreamModelID())
	if !isSafeUpstreamPathSegment(modelID) || model != prefix+modelID {
		return false
	}
	rawExpire, ok := entry["expireTime"].(string)
	if !ok {
		return false
	}
	if expiry, err := time.Parse(time.RFC3339Nano, rawExpire); err != nil || expiry.IsZero() {
		return false
	}
	usage, ok := entry["usageMetadata"].(map[string]any)
	if !ok {
		return false
	}
	number, ok := usage["totalTokenCount"].(json.Number)
	if !ok {
		return false
	}
	tokens, err := number.Int64()
	return err == nil && tokens > 0
}

func geminiCachedContentValidPageToken(token string) bool {
	if len(token) > geminiCachedContentRecoveryMaxToken || !utf8.ValidString(token) {
		return false
	}
	for _, r := range token {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func geminiCachedContentValidListQuery(raw string) bool {
	query, err := url.ParseQuery(raw)
	if err != nil || query.Encode() != raw || len(query["pageSize"]) != 1 || query.Get("pageSize") != "100" {
		return false
	}
	if len(query) == 1 {
		return true
	}
	return len(query) == 2 && len(query["pageToken"]) == 1 && query.Get("pageToken") != "" && geminiCachedContentValidPageToken(query.Get("pageToken"))
}
