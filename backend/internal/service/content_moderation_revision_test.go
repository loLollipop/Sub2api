package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type revisionModerationSettingRepo struct {
	*contentModerationRuntimeSettingRepo
	version     atomic.Uint64
	saveStarted chan<- struct{}
	saveWait    <-chan struct{}
	saveErr     error
	savedGroups []int64
}

func (r *revisionModerationSettingRepo) ContentModerationConfigVersion() uint64 {
	return r.version.Load()
}

func (r *revisionModerationSettingRepo) SetContentModerationConfig(_ context.Context, value string, groups []int64) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.mu.Lock()
	r.values[SettingKeyContentModerationConfig] = value
	r.savedGroups = append([]int64(nil), groups...)
	r.version.Add(1)
	r.mu.Unlock()
	if r.saveStarted != nil {
		r.saveStarted <- struct{}{}
	}
	if r.saveWait != nil {
		<-r.saveWait
	}
	return nil
}

func (r *revisionModerationSettingRepo) commit(value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[SettingKeyContentModerationConfig] = value
	r.version.Add(1)
}

func newRevisionModerationService(t *testing.T, ttl time.Duration) (*ContentModerationService, *revisionModerationSettingRepo) {
	t.Helper()
	repo := &revisionModerationSettingRepo{contentModerationRuntimeSettingRepo: &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled:      "true",
		SettingKeyContentModerationConfig: runtimeCacheTestConfig(t, "old-keyword"),
	}}}
	svc := runtimeCacheTestService(repo.contentModerationRuntimeSettingRepo, ttl)
	svc.settingRepo = repo
	return svc, repo
}

func TestContentModerationRuntimeRevisionBypassesFreshAndStaleSnapshots(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "stale"}[expired], func(t *testing.T) {
			svc, repo := newRevisionModerationService(t, time.Hour)
			initial, err := svc.loadRuntimeSnapshot(context.Background())
			require.NoError(t, err)
			if expired {
				old := *initial
				old.loadedAt = time.Now().Add(-2 * time.Hour)
				svc.runtimeSnapshot.Store(&old)
			}
			// A retry backoff may apply to TTL refreshes, but must not conceal a
			// committed scope mutation.
			svc.runtimeRefreshRetryAt.Store(time.Now().Add(time.Hour).UnixNano())
			repo.commit(runtimeCacheTestConfig(t, "new-keyword"))
			updated, err := svc.loadRuntimeSnapshot(context.Background())
			require.NoError(t, err)
			require.Equal(t, []string{"new-keyword"}, updated.config.BlockedKeywords)
			require.Equal(t, repo.ContentModerationConfigVersion(), updated.sourceVersion)
			_, calls := repo.calls()
			require.Equal(t, 2, calls)
		})
	}
}

func TestContentModerationRuntimeRevisionFailureDoesNotReturnOldScope(t *testing.T) {
	svc, repo := newRevisionModerationService(t, time.Hour)
	_, err := svc.loadRuntimeSnapshot(context.Background())
	require.NoError(t, err)
	repo.commit(runtimeCacheTestConfig(t, "new-keyword"))
	failure := errors.New("database unavailable")
	repo.failMultiple(failure)
	_, err = svc.loadRuntimeSnapshot(context.Background())
	require.ErrorIs(t, err, failure)
}

func TestContentModerationRuntimeRevisionRetriesBlockedOldLoad(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "refresh"}[existing], func(t *testing.T) {
			svc, repo := newRevisionModerationService(t, time.Hour)
			if existing {
				_, err := svc.loadRuntimeSnapshot(context.Background())
				require.NoError(t, err)
			}
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			repo.blockNextMultiple(started, release)
			done := make(chan *contentModerationRuntimeSnapshot, 1)
			errs := make(chan error, 1)
			go func() {
				svc.runtimeRefreshMu.Lock()
				defer svc.runtimeRefreshMu.Unlock()
				snapshot, err := svc.refreshRuntimeSnapshot(context.Background())
				done <- snapshot
				errs <- err
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("runtime query did not start")
			}
			repo.commit(runtimeCacheTestConfig(t, "new-keyword"))
			close(release)
			require.NoError(t, <-errs)
			snapshot := <-done
			require.Equal(t, []string{"new-keyword"}, snapshot.config.BlockedKeywords)
			require.Equal(t, repo.ContentModerationConfigVersion(), snapshot.sourceVersion)
			require.Same(t, snapshot, svc.runtimeSnapshot.Load())
			_, calls := repo.calls()
			if existing {
				require.Equal(t, 3, calls)
			} else {
				require.Equal(t, 2, calls)
			}
		})
	}
}

func TestContentModerationRuntimeRevisionDelayedSaveCannotRepublishPrunedScope(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty-cache", true: "warm-cache"}[initialized], func(t *testing.T) {
			testModerationDelayedSave(t, initialized)
		})
	}
}

func testModerationDelayedSave(t *testing.T, initialized bool) {
	t.Helper()
	svc, repo := newRevisionModerationService(t, time.Hour)
	if initialized {
		_, err := svc.loadRuntimeSnapshot(context.Background())
		require.NoError(t, err)
	}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	repo.saveStarted = started
	repo.saveWait = release
	done := make(chan error, 1)
	view := make(chan *ContentModerationConfigView, 1)
	go func() {
		groups := []int64{7}
		allGroups := true
		result, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{
			AllGroups: &allGroups,
			GroupIDs:  &groups,
		})
		view <- result
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("save did not commit")
	}
	// Save A has committed and is blocked on return. Deletion B now prunes
	// its retained IDs and commits before A can publish its response/cache.
	raw, err := repo.GetValue(context.Background(), SettingKeyContentModerationConfig)
	require.NoError(t, err)
	cfg, err := parseContentModerationConfig(raw)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, cfg.GroupIDs)
	cfg.GroupIDs = []int64{}
	updated, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo.commit(string(updated))
	close(release)
	require.NoError(t, <-done)
	require.Empty(t, (<-view).GroupIDs)
	require.Equal(t, []int64{7}, repo.savedGroups, "all_groups still retains IDs that must be locked")
	snapshot, err := svc.loadRuntimeSnapshot(context.Background())
	require.NoError(t, err)
	require.Empty(t, snapshot.config.GroupIDs)
	require.Equal(t, repo.ContentModerationConfigVersion(), snapshot.sourceVersion)
}

func TestContentModerationConfigAtomicWriterErrors(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "missing-group"}[missing], func(t *testing.T) {
			svc, repo := newRevisionModerationService(t, time.Hour)
			failure := errors.New("save failed")
			if missing {
				failure = ErrGroupNotFound
			}
			repo.saveErr = failure
			_, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{})
			if missing {
				require.Equal(t, "INVALID_CONTENT_MODERATION_GROUP", infraerrors.Reason(err))
			} else {
				require.ErrorIs(t, err, failure)
			}
			require.Zero(t, repo.ContentModerationConfigVersion())
		})
	}
}
