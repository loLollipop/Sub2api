package service

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"weak"

	"github.com/stretchr/testify/require"
)

func moderationBudgetService(capacity int) *ContentModerationService {
	return &ContentModerationService{asyncQueue: make(chan contentModerationTask, capacity)}
}

func drainModerationBudgetQueue(t *testing.T, svc *ContentModerationService) {
	t.Helper()
	for len(svc.asyncQueue) > 0 {
		task, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
		require.True(t, ok)
		svc.releaseAsyncTask(task)
	}
	require.Zero(t, svc.asyncPayloadBytes.Load())
}

func TestContentModerationQueueBudgetMixedConcurrentProducers(t *testing.T) {
	svc := moderationBudgetService(1024)
	cfg := defaultContentModerationConfig()
	cfg.QueueSize = 1024
	text := strings.Repeat("界", maxModerationLogExcerptRunes)
	// Seed both task kinds so scheduling cannot starve either kind from the
	// shared budget before the concurrent admission exercise starts.
	svc.enqueueAsync(ContentModerationCheckInput{logText: text}, cfg, ContentModerationInput{Text: "audit", Images: []string{text}}, "hash")
	svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{InputExcerpt: text}, "hash", false, false)
	var producers sync.WaitGroup
	for i := 0; i < 8; i++ {
		producers.Add(1)
		go func(audit bool) {
			defer producers.Done()
			for j := 0; j < 24; j++ {
				if audit {
					svc.enqueueAsync(ContentModerationCheckInput{logText: text}, cfg, ContentModerationInput{Text: "audit", Images: []string{text}}, "hash")
				} else {
					svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{InputExcerpt: text}, "hash", false, false)
				}
			}
		}(i%2 == 0)
	}
	producers.Wait()
	require.Positive(t, svc.asyncDropped.Load())
	require.LessOrEqual(t, svc.asyncPayloadBytes.Load(), int64(maxContentModerationAsyncPayloadBytes))
	used := svc.asyncPayloadBytes.Load()
	require.Equal(t, int64(194), svc.asyncEnqueued.Load()+svc.asyncDropped.Load())
	var charged int64
	var audits, records int
	for len(svc.asyncQueue) > 0 {
		task, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
		require.True(t, ok)
		charged += task.payloadBytes
		if task.log == nil {
			audits++
			require.Equal(t, text, task.input.logText)
		} else {
			records++
			require.Equal(t, text, task.log.InputExcerpt)
		}
		// Dequeued payload remains charged until the worker finishes with it.
		svc.releaseAsyncTask(task)
	}
	require.Positive(t, charged)
	require.Equal(t, used, charged)
	require.Positive(t, audits)
	require.Positive(t, records)
	require.Zero(t, svc.asyncPayloadBytes.Load())
	svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{InputExcerpt: text}, "hash", false, false)
	require.Len(t, svc.asyncQueue, 1, "completed tasks must make capacity available again")
	drainModerationBudgetQueue(t, svc)
}

func TestContentModerationQueueBudgetRefundsRejectedEnqueue(t *testing.T) {
	t.Run("oversized audit image and frozen config", func(t *testing.T) {
		svc := moderationBudgetService(1)
		cfg := defaultContentModerationConfig()
		large := strings.Repeat("x", maxContentModerationAsyncPayloadBytes)
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{Images: []string{large}}, "hash")
		cfg.APIKeys = []string{large}
		svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{}, "hash", false, false)
		require.Empty(t, svc.asyncQueue)
		require.Equal(t, int64(2), svc.asyncDropped.Load())
		require.Zero(t, svc.asyncPayloadBytes.Load())
	})
	t.Run("physical channel full", func(t *testing.T) {
		svc := moderationBudgetService(1)
		cfg := defaultContentModerationConfig()
		svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{InputExcerpt: "first"}, "hash", false, false)
		used := svc.asyncPayloadBytes.Load()
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{Text: "second"}, "hash")
		require.Equal(t, used, svc.asyncPayloadBytes.Load())
		require.Equal(t, int64(1), svc.asyncDropped.Load())
		drainModerationBudgetQueue(t, svc)
	})
	t.Run("closed channel drains and rejects sends", func(t *testing.T) {
		svc := moderationBudgetService(2)
		cfg := defaultContentModerationConfig()
		svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{}, "hash", false, false)
		used := svc.asyncPayloadBytes.Load()
		close(svc.asyncQueue)
		require.NotPanics(t, func() {
			svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{Text: "closed"}, "hash")
		})
		require.Equal(t, used, svc.asyncPayloadBytes.Load())
		drainModerationBudgetQueue(t, svc)
		_, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
		require.False(t, ok)
		svc.enqueueRecord(ContentModerationCheckInput{}, cfg, &ContentModerationLog{}, "hash", false, false)
		require.Equal(t, int64(2), svc.asyncDropped.Load())
		require.Zero(t, svc.asyncPayloadBytes.Load())
	})
}

func TestContentModerationAuditQueueReleasesBodyAndPreservesFullLog(t *testing.T) {
	svc := moderationBudgetService(1)
	text := strings.Repeat("界", maxModerationLogExcerptRunes)
	body := func() weak.Pointer[byte] {
		input, body := moderationLifecycleInput()
		input.Body = append(input.Body[:len(input.Body)-len(`blocked prompt"}]}`)], []byte(text+`"}]}`)...)
		// append can reallocate: track the backing array actually enqueued.
		body = weak.Make(&input.Body[0])
		svc.enqueueAsync(input, defaultContentModerationConfig(), ContentModerationInput{Text: "bounded audit input"}, "hash")
		return body
	}()
	defer runtime.KeepAlive(svc)
	require.Len(t, svc.asyncQueue, 1)
	requireModerationBodyCollected(t, body)
	task, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
	require.True(t, ok)
	defer svc.releaseAsyncTask(task)
	require.Nil(t, task.input.Body)
	require.Equal(t, text, task.input.logText)
	log := svc.buildLog(task.input, task.config, ContentModerationActionAllow, false, "", 0, nil, task.content.Text, nil, nil, "")
	require.Equal(t, text, log.InputExcerpt)
}

func TestContentModerationAuditQueueReleasesParsedModelBody(t *testing.T) {
	svc := moderationBudgetService(1)
	body := func() weak.Pointer[byte] {
		input, body := moderationLifecycleInput()
		svc.enqueueAsync(input, defaultContentModerationConfig(), ContentModerationInput{Text: "bounded audit input"}, "hash")
		return body
	}()
	defer runtime.KeepAlive(svc)
	require.Len(t, svc.asyncQueue, 1)
	requireModerationBodyCollected(t, body)
	task, ok := svc.dequeueAsyncTask(context.Background(), time.Second)
	require.True(t, ok)
	defer svc.releaseAsyncTask(task)
	require.Equal(t, "claude-sonnet-4-5", task.input.Model)
	require.Nil(t, task.input.Body)
}

type moderationBudgetBlockingRepo struct {
	contentModerationTestRepo
	started       chan struct{}
	resume        chan struct{}
	panicOnCreate bool
}

func (r *moderationBudgetBlockingRepo) CreateLog(ctx context.Context, log *ContentModerationLog) error {
	close(r.started)
	<-r.resume
	if r.panicOnCreate {
		panic("test worker budget cleanup")
	}
	return r.contentModerationTestRepo.CreateLog(ctx, log)
}

func TestContentModerationQueueBudgetWorkerLifetime(t *testing.T) {
	for _, panicOnCreate := range []bool{false, true} {
		name := "success"
		if panicOnCreate {
			name = "panic"
		}
		t.Run(name, func(t *testing.T) {
			svc := moderationBudgetService(2)
			repo := &moderationBudgetBlockingRepo{started: make(chan struct{}), resume: make(chan struct{}), panicOnCreate: panicOnCreate}
			svc.repo = repo
			svc.settingRepo = &contentModerationRuntimeSettingRepo{values: map[string]string{
				SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: runtimeCacheTestConfig(t),
			}}
			svc.enqueueRecord(ContentModerationCheckInput{}, defaultContentModerationConfig(), &ContentModerationLog{InputExcerpt: strings.Repeat("界", maxModerationLogExcerptRunes)}, "hash", false, false)
			used := svc.asyncPayloadBytes.Load()
			done := make(chan struct{})
			go func() { svc.worker(0); close(done) }()
			select {
			case <-repo.started:
			case <-time.After(3 * time.Second):
				t.Fatal("record worker did not start")
			}
			require.Empty(t, svc.asyncQueue)
			require.Equal(t, used, svc.asyncPayloadBytes.Load(), "running tasks retain their reservation")
			close(svc.asyncQueue)
			close(repo.resume)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not exit after draining closed queue")
			}
			require.Zero(t, svc.asyncPayloadBytes.Load())
			require.Zero(t, svc.asyncActive.Load())
			if !panicOnCreate {
				require.Equal(t, int64(1), svc.asyncProcessed.Load())
			}
		})
	}
}

func TestContentModerationQueueBudgetWorkerReleasesRevokedAudit(t *testing.T) {
	svc := moderationBudgetService(1)
	svc.settingRepo = &contentModerationRuntimeSettingRepo{values: map[string]string{
		SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: runtimeCacheTestConfig(t),
	}}
	// This frozen observe task no longer matches the current pre-block
	// keyword-only configuration, so dispatch exits before any HTTP request.
	cfg := defaultContentModerationConfig()
	cfg.Mode = ContentModerationModeObserve
	svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{Text: "revoked audit"}, "hash")
	require.Positive(t, svc.asyncPayloadBytes.Load())
	close(svc.asyncQueue)
	done := make(chan struct{})
	go func() { svc.worker(0); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not drain revoked audit task")
	}
	require.Zero(t, svc.asyncPayloadBytes.Load())
	require.Zero(t, svc.asyncProcessed.Load())
	require.Zero(t, svc.asyncActive.Load())
}
