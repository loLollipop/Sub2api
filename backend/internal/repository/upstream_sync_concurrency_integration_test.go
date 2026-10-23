//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestInstallationKeyIntegrationConcurrentFirstBoot(t *testing.T) {
	const count = 8
	configs := make([]*config.Config, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for i := range count {
		configs[i] = &config.Config{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = ensureTOTPInstallationKey(context.Background(), integrationEntClient, configs[i])
		}(i)
	}
	wg.Wait()
	for i := range count {
		require.NoError(t, errs[i])
		require.True(t, configs[i].Totp.EncryptionKeyPersistent)
		require.Equal(t, configs[0].Totp.EncryptionKey, configs[i].Totp.EncryptionKey)
	}
	writer, err := NewAESEncryptor(configs[0])
	require.NoError(t, err)
	ciphertext, err := writer.Encrypt("postgres-restart-canary")
	require.NoError(t, err)
	restarted := &config.Config{}
	require.NoError(t, ensureTOTPInstallationKey(context.Background(), integrationEntClient, restarted))
	reader, err := NewAESEncryptor(restarted)
	require.NoError(t, err)
	plaintext, err := reader.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "postgres-restart-canary", plaintext)
}

func TestMediaBillingJobIntegrationReconcilePrepareFailRace(t *testing.T) {
	for range 12 {
		ctx := context.Background()
		f := newMediaBillingIntegrationFixture(t, 10)
		job := f.videoJob(fmt.Sprintf("media-race-%d", time.Now().UnixNano()), 2)
		_, err := f.repo.CreateMediaBillingJob(ctx, job)
		require.NoError(t, err)
		upstreamID := "upstream-" + job.ID
		require.NoError(t, f.repo.SubmitMediaBillingJob(ctx, job.ID, upstreamID, []byte(`{}`)))
		start := make(chan struct{})
		var wg sync.WaitGroup
		var prepareErr, failErr, reconcileErr error
		for _, run := range []func(){
			func() { _, prepareErr = f.repo.PrepareMediaBillingIntent(ctx, job.ID, f.intent("race:"+job.ID, 1.5)) },
			func() { _, failErr = f.repo.FailMediaBillingJob(ctx, job.ID) },
			func() { reconcileErr = f.repo.ReconcileMediaBillingJob(ctx, job.ID) },
		} {
			wg.Add(1)
			go func(run func()) { defer wg.Done(); <-start; run() }(run)
		}
		close(start)
		wg.Wait()
		require.NoError(t, reconcileErr)
		if prepareErr == nil {
			require.ErrorIs(t, failErr, service.ErrUsageBillingRequestConflict)
			for range 2 {
				_, err = f.repo.ApplyMediaBillingJob(ctx, job.ID)
				require.NoError(t, err)
			}
			require.NoError(t, f.repo.CompleteMediaBillingJob(ctx, job.ID))
			assertMediaBillingWallet(t, ctx, f.user.ID, 8.5, 0)
			assertMediaBillingOutboxDeltas(t, ctx, f.user.ID, []float64{-2, 0.5})
		} else {
			require.ErrorIs(t, prepareErr, service.ErrUsageBillingRequestConflict)
			require.NoError(t, failErr)
			_, err = f.repo.FailMediaBillingJob(ctx, job.ID)
			require.NoError(t, err)
			assertMediaBillingWallet(t, ctx, f.user.ID, 10, 0)
			assertMediaBillingOutboxDeltas(t, ctx, f.user.ID, []float64{-2, 2})
		}
	}
}
