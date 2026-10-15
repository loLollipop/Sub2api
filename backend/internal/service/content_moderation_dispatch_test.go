package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dispatchTestConfig(t *testing.T, repo *contentModerationRuntimeSettingRepo, cfg *ContentModerationConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, repo.Set(context.Background(), SettingKeyContentModerationConfig, string(raw)))
}

func dispatchTestService(t *testing.T, base string, engine string) (*ContentModerationService, *contentModerationRuntimeSettingRepo, *ContentModerationConfig, ContentModerationCheckInput) {
	t.Helper()
	cfg := defaultContentModerationConfig()
	cfg.Enabled, cfg.Mode, cfg.Engine = true, ContentModerationModeObserve, engine
	cfg.APIKeys, cfg.BaseURL, cfg.RetryCount = []string{"private-dispatch-key", "retry-key"}, base, 0
	cfg.AllGroups, cfg.GroupIDs = false, []int64{7}
	cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"business-model"}}
	if engine == ContentModerationEngineTypeSafe {
		cfg.TypeSafe = moderationEngineDefaults(engine)
		cfg.TypeSafe.BaseURL, cfg.TypeSafe.APIKeys, cfg.TypeSafe.RetryCount = base, cfg.APIKeys, 0
	}
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true"}}
	dispatchTestConfig(t, repo, cfg)
	svc := runtimeCacheTestService(repo, time.Hour)
	svc.httpClient = http.DefaultClient
	svc.asyncQueue = make(chan contentModerationTask, 4)
	_, err := svc.loadRuntimeSnapshot(context.Background())
	require.NoError(t, err)
	group := int64(7)
	input := ContentModerationCheckInput{GroupID: &group, Model: "business-model"}
	return svc, repo, cfg, input
}

func dispatchTestEnqueue(t *testing.T, svc *ContentModerationService, cfg *ContentModerationConfig, input ContentModerationCheckInput) contentModerationTask {
	t.Helper()
	content := ContentModerationInput{Text: "private user text"}
	svc.enqueueAsync(input, cfg.effectiveEngine(cfg.Engine), content, content.Hash())
	task, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
	require.True(t, ok)
	return task
}

func TestContentModerationDequeuedTaskReauthorizesPersistedGates(t *testing.T) {
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		for _, change := range []string{"unchanged", "global_off", "content_off", "mode_off", "engine", "destination", "key", "proxy", "group", "model", "audit_model", "sample", "settings_failure", "malformed"} {
			t.Run(engine+"/"+change, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.Equal(t, "Bearer private-dispatch-key", r.Header.Get("Authorization"))
					if engine == ContentModerationEngineTypeSafe {
						answers := map[string]any{}
						for _, category := range ContentModerationCategories() {
							answers[category] = map[string]any{"type": "noul", "noul": 0.1}
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-actual", "answers": answers})
					} else {
						_, _ = w.Write([]byte(`{"model":"omni-actual","results":[{"flagged":false,"category_scores":{}}]}`))
					}
				}))
				defer server.Close()
				svc, repo, cfg, input := dispatchTestService(t, server.URL, engine)
				task := dispatchTestEnqueue(t, svc, cfg, input)
				switch change {
				case "global_off":
					require.NoError(t, repo.Set(context.Background(), SettingKeyRiskControlEnabled, "false"))
				case "content_off":
					cfg.Enabled = false
				case "mode_off":
					cfg.Mode = ContentModerationModeOff
				case "engine":
					if cfg.Engine == ContentModerationEngineOpenAI {
						cfg.Engine = ContentModerationEngineTypeSafe
					} else {
						cfg.Engine = ContentModerationEngineOpenAI
					}
				case "destination":
					if cfg.TypeSafe != nil {
						cfg.TypeSafe.BaseURL += "/changed"
					} else {
						cfg.BaseURL += "/changed"
					}
				case "key":
					if cfg.TypeSafe != nil {
						cfg.TypeSafe.APIKeys = []string{"new-key"}
					} else {
						cfg.APIKeys = []string{"new-key"}
					}
				case "proxy":
					id := int64(9)
					if cfg.TypeSafe != nil {
						cfg.TypeSafe.ProxyID = &id
					} else {
						cfg.ProxyID = &id
					}
				case "group":
					cfg.GroupIDs = []int64{8}
				case "model":
					cfg.ModelFilter.Models = []string{"another-model"}
				case "audit_model":
					if cfg.TypeSafe != nil {
						cfg.TypeSafe.Model = "new-audit-model"
					} else {
						cfg.Model = "new-audit-model"
					}
				case "sample":
					cfg.SampleRate = 0
				case "settings_failure":
					repo.failMultiple(errors.New("secret DB error"))
				}
				dispatchTestConfig(t, repo, cfg)
				if change == "malformed" {
					require.NoError(t, repo.Set(context.Background(), SettingKeyContentModerationConfig, "bad"))
				}
				// The previously loaded one-hour snapshot deliberately remains stale.
				svc.dispatchAsyncTask(context.Background(), task)
				if change == "unchanged" {
					require.Equal(t, int32(1), calls.Load())
				} else {
					require.Zero(t, calls.Load())
				}
			})
		}
	}
}

func TestContentModerationDequeuedRetryRevocation(t *testing.T) {
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		t.Run(engine, func(t *testing.T) {
			var calls atomic.Int32
			var revoke func()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				time.AfterFunc(25*time.Millisecond, revoke)
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("private user text private-dispatch-key"))
			}))
			defer server.Close()
			svc, repo, cfg, input := dispatchTestService(t, server.URL, engine)
			cfg.RetryCount = 1
			if cfg.TypeSafe != nil {
				cfg.TypeSafe.RetryCount = 1
			}
			dispatchTestConfig(t, repo, cfg)
			task := dispatchTestEnqueue(t, svc, cfg, input)
			revoke = func() { require.NoError(t, repo.Set(context.Background(), SettingKeyRiskControlEnabled, "false")) }
			svc.dispatchAsyncTask(context.Background(), task)
			require.Equal(t, int32(1), calls.Load())
			statuses := svc.apiKeyStatuses(cfg.APIKeys, engine)
			for _, status := range statuses {
				require.NotContains(t, status.LastError, "private")
			}
		})
	}
}

type delayedModerationProxyRepo struct {
	ProxyRepository
	started chan struct{}
	release chan struct{}
	proxy   *Proxy
}

func (r *delayedModerationProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	close(r.started)
	<-r.release
	return r.proxy, nil
}

func TestContentModerationDequeuedRevocationAfterProxyResolution(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(address.Port())
	require.NoError(t, err)
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		t.Run(engine, func(t *testing.T) {
			svc, repo, cfg, input := dispatchTestService(t, "http://moderation.invalid", engine)
			id := int64(7)
			if cfg.TypeSafe != nil {
				cfg.TypeSafe.ProxyID = &id
			} else {
				cfg.ProxyID = &id
			}
			dispatchTestConfig(t, repo, cfg)
			proxy := &delayedModerationProxyRepo{started: make(chan struct{}), release: make(chan struct{}), proxy: &Proxy{ID: id, Protocol: "http", Host: address.Hostname(), Port: port, Status: StatusActive}}
			svc.proxyRepo = proxy
			task := dispatchTestEnqueue(t, svc, cfg, input)
			done := make(chan struct{})
			go func() { svc.dispatchAsyncTask(context.Background(), task); close(done) }()
			select {
			case <-proxy.started:
			case <-time.After(3 * time.Second):
				t.Fatal("proxy not resolved")
			}
			require.NoError(t, repo.Set(context.Background(), SettingKeyRiskControlEnabled, "false"))
			close(proxy.release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("dispatch did not finish")
			}
			require.Zero(t, calls.Load())
		})
	}
}

func TestContentModerationDefaultEngineAndSavingRemainDisabled(t *testing.T) {
	repo := &contentModerationTestSettingRepo{values: map[string]string{}}
	svc := &ContentModerationService{settingRepo: repo}
	view, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.Equal(t, ContentModerationEngineOpenAI, view.Engine)
	engine := ContentModerationEngineTypeSafe
	keys := []string{"secret-typesafe-key"}
	view, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{Engine: &engine, APIKeys: &keys})
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.Empty(t, repo.values[SettingKeyRiskControlEnabled])
	require.Len(t, view.EngineConfigs[ContentModerationEngineOpenAI].APIKeyStatuses, 0)
}

type dispatchFreshSettings struct {
	*contentModerationRuntimeSettingRepo
	fresh    map[string]string
	freshErr error
	reads    int
}

func (r *dispatchFreshSettings) GetContentModerationDispatchSettings(context.Context) (map[string]string, error) {
	r.reads++
	return r.fresh, r.freshErr
}

func TestContentModerationDequeuedTaskUsesUncachedDispatchInterface(t *testing.T) {
	svc, cached, cfg, input := dispatchTestService(t, "https://must-not-dispatch.invalid", ContentModerationEngineOpenAI)
	task := dispatchTestEnqueue(t, svc, cfg, input)
	fresh := &dispatchFreshSettings{contentModerationRuntimeSettingRepo: cached, fresh: map[string]string{SettingKeyRiskControlEnabled: "false"}}
	svc.settingRepo = fresh
	_, before := cached.calls()
	svc.dispatchAsyncTask(context.Background(), task)
	_, after := cached.calls()
	require.Equal(t, before, after, "cached GetMultiple must not authorize dispatch")
	require.Equal(t, 1, fresh.reads)
}
