//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokRelayCompletedSettlementPreservesFrozenPriceOnce(t *testing.T) {
	svc, repo, input, pending := newMediaBillingServiceTest(t)
	input.APIKey.Group.VideoModelPrices[VideoPriceFamilyGrokImagineVideo]["720p"] = 0.05
	input.APIKey.Group.VideoRateMultiplier = 0.5
	ctx := context.Background()
	job, err := svc.CreateGrokVideoBillingJob(ctx, input, pending)
	require.NoError(t, err)
	require.InDelta(t, 0.15, job.ReservedAmount, 1e-8)
	require.NoError(t, svc.SubmitGrokVideoBillingJob(ctx, job, "relay-frozen", pending))
	input.APIKey.Group.VideoModelPrices[VideoPriceFamilyGrokImagineVideo]["720p"] = 1
	input.APIKey.Group.VideoRateMultiplier = 9
	job, err = repo.GetMediaBillingJob(ctx, input.User.ID, input.APIKey.ID, "relay-frozen")
	require.NoError(t, err)
	result := ExtractGrokVideoBillingFromStatusBody([]byte(`{"status":"completed","data":[{"url":"https://example.test/v.mp4"}],"seconds":6}`), &pending, "relay-frozen")
	require.NotNil(t, result)
	for range 2 {
		require.NoError(t, svc.SettleGrokVideoBillingJob(ctx, job, result))
	}
	require.Equal(t, 1, repo.applied)
	require.InDelta(t, 99.85, repo.balance, 1e-8)
	stored, err := repo.GetMediaBillingJobByID(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, MediaBillingDone, stored.Status)
	require.InDelta(t, 0.15, stored.Intent.Command.BalanceCost, 1e-8)
}
