package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"time"
)

type moderationDispatchKey struct{}
type moderationDispatch struct {
	config *ContentModerationConfig
	input  ContentModerationCheckInput
	hash   string
}

var errModerationDispatchRevoked = errors.New("moderation dispatch no longer authorized")

func withModerationDispatch(ctx context.Context, cfg *ContentModerationConfig, input ContentModerationCheckInput, hash string) context.Context {
	input.GroupID = cloneInt64Ptr(input.GroupID)
	return context.WithValue(ctx, moderationDispatchKey{}, moderationDispatch{cloneContentModerationConfig(cfg), input, hash})
}

// Only automatic checks carry this context. The administrator's explicit test
// remains an opt-in request, independent of the production enablement gates.
func (s *ContentModerationService) authorizeModerationDispatch(ctx context.Context) error {
	dispatch, automatic := ctx.Value(moderationDispatchKey{}).(moderationDispatch)
	if !automatic {
		return nil
	}
	if dispatch.config == nil || s.settingRepo == nil || ctx.Err() != nil {
		return errModerationDispatchRevoked
	}
	var values map[string]string
	var err error
	if fresh, ok := s.settingRepo.(interface {
		GetContentModerationDispatchSettings(context.Context) (map[string]string, error)
	}); ok {
		values, err = fresh.GetContentModerationDispatchSettings(ctx)
	} else {
		values, err = s.settingRepo.GetMultiple(ctx, []string{SettingKeyRiskControlEnabled, SettingKeyContentModerationConfig})
	}
	if err != nil || values[SettingKeyRiskControlEnabled] != "true" {
		return errModerationDispatchRevoked
	}
	raw := values[SettingKeyContentModerationConfig]
	var persisted ContentModerationConfig
	if json.Unmarshal([]byte(raw), &persisted) != nil || !persisted.Enabled || (persisted.Mode != ContentModerationModeObserve && persisted.Mode != ContentModerationModePreBlock) {
		return errModerationDispatchRevoked
	}
	if persisted.ModelFilter.Type != "" && persisted.ModelFilter.Type != ContentModerationModelFilterAll && persisted.ModelFilter.Type != ContentModerationModelFilterInclude && persisted.ModelFilter.Type != ContentModerationModelFilterExclude {
		return errModerationDispatchRevoked
	}
	current, err := parseContentModerationConfig(raw)
	if err != nil || !current.Enabled || (current.Mode != ContentModerationModeObserve && current.Mode != ContentModerationModePreBlock) || !current.includesGroup(dispatch.input.GroupID) || !current.includesModel(dispatch.input.Model) || !current.shouldSample(dispatch.hash) {
		return errModerationDispatchRevoked
	}
	if current.Mode == ContentModerationModePreBlock && current.KeywordBlockingMode == ContentModerationKeywordModeKeywordOnly {
		return errModerationDispatchRevoked
	}
	frozen := dispatch.config
	if current.Engine != frozen.Engine || !validModerationEngine(current.Engine) || !reflect.DeepEqual(current.engineProfile(current.Engine), frozen.engineProfile(frozen.Engine)) {
		return errModerationDispatchRevoked
	}
	profile := current.engineProfile(current.Engine)
	endpoint, err := url.Parse(profile.BaseURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || len(profile.APIKeys) == 0 {
		return errModerationDispatchRevoked
	}
	return nil
}

type moderationDispatchTransport struct {
	service *ContentModerationService
	next    http.RoundTripper
}

func (t moderationDispatchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// This runs after resolving the proxy and after every retry wait, immediately
	// before the credential-bearing request enters the underlying transport.
	if err := t.service.authorizeModerationDispatch(req.Context()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

func (s *ContentModerationService) dispatchAsyncTask(ctx context.Context, task contentModerationTask) {
	// Pre-backport audit tasks have no frozen configuration: never retarget them.
	if task.config == nil {
		return
	}
	ctx = withModerationDispatch(ctx, task.config, task.input, task.inputHash)
	if s.authorizeModerationDispatch(ctx) != nil {
		return
	}
	s.asyncActive.Add(1)
	defer s.asyncActive.Add(-1)
	queueDelay := int(time.Since(task.enqueuedAt).Milliseconds())
	_ = s.checkSync(ctx, task.input, task.config, task.content, task.inputHash, &queueDelay, false)
	s.asyncProcessed.Add(1)
}
