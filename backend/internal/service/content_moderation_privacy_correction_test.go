package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/stretchr/testify/require"
)

func TestContentModerationReturnedModelCannotEchoPrivateMetadata(t *testing.T) {
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		for _, model := range []string{"private-active-key", "jev-private-active-key-version", "private-other-engine-key", "private-user-input", "jev-private-user-input-version", "PRIVATE-ACTIVE-KEY", "jev-PRIVATE-ACTIVE-KEY-version", "PRIVATE-OTHER-ENGINE-KEY", "PRIVATE-USER-INPUT", "jev-PRIVATE-USER-INPUT-version", "https://private.example/secret", "customer-ssn-123456789", "jev-customer-ssn-123456789-version", "CUSTOMER-SSN-123456789", "jev-CUSTOMER-SSN-123456789-version"} {
			t.Run(engine+"/"+model, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if engine == ContentModerationEngineOpenAI {
						_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "results": []any{map[string]any{"category_scores": map[string]float64{"sexual": 0.9}}}})
						return
					}
					answers := map[string]any{}
					for _, category := range ContentModerationCategories() {
						answers[category] = map[string]any{"type": "noul", "noul": 0.9}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "answers": answers})
				}))
				defer server.Close()
				cfg := defaultContentModerationConfig()
				cfg.Engine, cfg.BaseURL, cfg.RetryCount = engine, server.URL, 0
				cfg.APIKeys = []string{"private-active-key"}
				cfg.TypeSafe = moderationEngineDefaults(ContentModerationEngineTypeSafe)
				cfg.TypeSafe.APIKeys = []string{"private-other-engine-key"}
				svc := &ContentModerationService{httpClient: server.Client()}
				text := "private-user-input"
				if strings.Contains(strings.ToLower(model), "customer-ssn-") {
					text = "customer-ssn-123456789 and account review"
				}
				result, err := svc.callModeration(context.Background(), cfg, text)
				require.NoError(t, err, "unsafe metadata must not discard valid audit scores")
				require.NotNil(t, result)
				require.Empty(t, result.EngineMeta.Model)
				flagged, _, _ := evaluateModerationScores(result.CategoryScores, cfg.Thresholds)
				require.True(t, flagged, "metadata sanitizing must not turn a hit into fail-open")
			})
		}
	}
}

func TestContentModerationTypeSafeMissingModelRetainsBlockingScores(t *testing.T) {
	for _, model := range []any{nil, "", " "} {
		t.Run(fmt.Sprint(model), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				answers := map[string]any{}
				for _, category := range ContentModerationCategories() {
					answers[category] = map[string]any{"type": "noul", "noul": 0.99}
				}
				response := map[string]any{"answers": answers}
				if model != nil {
					response["model"] = model
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			cfg := defaultContentModerationConfig()
			cfg.Engine, cfg.BaseURL, cfg.Mode = ContentModerationEngineTypeSafe, server.URL, ContentModerationModePreBlock
			cfg.APIKeys, cfg.RetryCount = []string{"test-key"}, 0
			repo := &contentModerationTestRepo{}
			svc := &ContentModerationService{httpClient: server.Client(), repo: repo}
			result, err := svc.callModeration(context.Background(), cfg, "需要审核的文字")
			require.NoError(t, err)
			require.Empty(t, result.EngineMeta.Model)
			decision := svc.checkSync(context.Background(), ContentModerationCheckInput{}, cfg, ContentModerationInput{Text: "需要审核的文字"}, "hash", nil, true)
			require.False(t, decision.Allowed)
			require.True(t, decision.Blocked)
			require.Equal(t, ContentModerationActionBlock, decision.Action)
		})
	}
}

func TestContentModerationOpenAIImageModelEchoIsNotRecorded(t *testing.T) {
	const identifier = "Customer-SSN-123456789"
	for _, text := range []string{"", "需要审核的文字"} {
		for _, image := range []string{
			"https://private.invalid/" + identifier + ".png",
			"https://private.invalid/image.png?token=" + identifier,
			"https://private.invalid/image.png?token=Customer%2DSSN%2D123456789",
			"https://Customer-SSN-123456789@private.invalid/image.png",
			"https://user:Customer%2DSSN%2D123456789@private.invalid/image.png",
		} {
			t.Run(text+"/"+image, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request moderationAPIRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					wire, err := json.Marshal(request.Input)
					require.NoError(t, err)
					require.Contains(t, string(wire), image, "image sampling/wire URL must remain unchanged")
					_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-customer-ssn-123456789-version", "results": []any{map[string]any{"category_scores": map[string]float64{"sexual": 0.99}}}})
				}))
				defer server.Close()
				cfg := defaultContentModerationConfig()
				cfg.BaseURL, cfg.APIKeys, cfg.RetryCount = server.URL, []string{"test-key"}, 0
				svc := &ContentModerationService{httpClient: server.Client()}
				input := ContentModerationInput{Text: text, Images: []string{image}}
				result, err := svc.callModeration(context.Background(), cfg, moderationInputForEngine(ContentModerationEngineOpenAI, input))
				require.NoError(t, err)
				require.Empty(t, result.EngineMeta.Model)
				require.Equal(t, 0.99, result.CategoryScores["sexual"])
			})
		}
	}
}

func TestContentModerationTypeSafeImageModelEchoIsNotRecorded(t *testing.T) {
	for _, image := range []string{
		"https://private.invalid/Customer-SSN-123456789.png",
		"https://private.invalid/image.png?token=Customer-SSN-123456789",
		"https://private.invalid/image.png?token=Customer%2DSSN%2D123456789",
		"https://Customer-SSN-123456789@private.invalid/image.png",
		"https://user:Customer%2DSSN%2D123456789@private.invalid/image.png",
	} {
		t.Run(image, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request typesafe.Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, "需要审核的文字", request.State)
				wire, err := json.Marshal(request)
				require.NoError(t, err)
				require.NotContains(t, string(wire), "private.invalid")
				require.NotContains(t, string(wire), "123456789", "comparison-only identifiers must never be sent")
				answers := map[string]any{}
				for _, category := range ContentModerationCategories() {
					answers[category] = map[string]any{"type": "noul", "noul": 0.99}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-customer-ssn-123456789-version", "answers": answers})
			}))
			defer server.Close()
			cfg := defaultContentModerationConfig()
			cfg.Engine, cfg.BaseURL, cfg.Mode = ContentModerationEngineTypeSafe, server.URL, ContentModerationModePreBlock
			cfg.APIKeys, cfg.RetryCount = []string{"test-key"}, 0
			repo := &contentModerationTestRepo{}
			svc := &ContentModerationService{httpClient: server.Client(), repo: repo, asyncQueue: make(chan contentModerationTask, 1)}
			input := ContentModerationInput{Text: "需要审核的文字", Images: []string{"https://private.invalid/first.png", image}}
			decision := svc.checkSync(context.Background(), ContentModerationCheckInput{}, cfg, input, "hash", nil, true)
			require.True(t, decision.Blocked, "unsafe metadata must not discard valid blocking scores")
			require.Len(t, svc.asyncQueue, 1, "pre-block logging is deferred to the record worker")
			task := <-svc.asyncQueue
			require.NotNil(t, task.log)
			svc.persistContentModerationLog(context.Background(), task.config, task.log, task.inputHash, task.recordHash, task.applySideEffects)
			require.Len(t, repo.logs, 1)
			require.Empty(t, repo.logs[0].EngineMeta.Model)
			require.Equal(t, 2, repo.logs[0].EngineMeta.SkippedImages)
		})
	}
}

func TestContentModerationTypeSafeCountsAllUnauditedImages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request typesafe.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "需要审核的文字", request.State)
		raw, err := json.Marshal(request)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private.invalid")
		answers := map[string]any{}
		for _, category := range ContentModerationCategories() {
			answers[category] = map[string]any{"type": "noul", "noul": 0.1}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-fixed", "answers": answers})
	}))
	defer server.Close()
	cfg := defaultContentModerationConfig()
	cfg.Engine, cfg.BaseURL, cfg.RetryCount = ContentModerationEngineTypeSafe, server.URL, 0
	cfg.Mode, cfg.RecordNonHits = ContentModerationModeObserve, true
	cfg.APIKeys = []string{"test-key"}
	content := ContentModerationInput{Text: "需要审核的文字", Images: []string{
		"https://private.invalid/1.png", "https://private.invalid/2.png", "https://private.invalid/3.png", " https://private.invalid/1.png ", "",
	}}
	repo := &contentModerationTestRepo{}
	svc := &ContentModerationService{httpClient: server.Client(), repo: repo}
	queueDelay := 0
	decision := svc.checkSync(context.Background(), ContentModerationCheckInput{}, cfg, content, "hash", &queueDelay, false)
	require.True(t, decision.Allowed)
	require.Equal(t, int32(1), calls.Load())
	require.Len(t, repo.logs, 1)
	require.Equal(t, 3, repo.logs[0].EngineMeta.SkippedImages)
	require.Equal(t, "jev-fixed", repo.logs[0].EngineMeta.Model)

	content.Text = ""
	decision = svc.checkSync(context.Background(), ContentModerationCheckInput{}, cfg, content, "hash", &queueDelay, false)
	require.True(t, decision.Allowed)
	require.Equal(t, int32(1), calls.Load(), "pure images must never be sent")
	require.Len(t, repo.logs, 2)
	require.Equal(t, ContentModerationActionError, repo.logs[1].Action)
	require.Equal(t, 3, repo.logs[1].EngineMeta.SkippedImages)
	require.Empty(t, repo.logs[1].EngineMeta.Model)

	// The OpenAI sampling contract remains one image, independent of TypeSafe.
	_, count := typeSafeText(moderationInputForEngine(ContentModerationEngineOpenAI, content))
	require.Equal(t, 1, count)
}
