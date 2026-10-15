//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type adminUserGroupRateCaptureRepo struct {
	UserGroupRateRepository
	userID  int64
	rates   map[int64]*float64
	group   int64
	entries []GroupRateMultiplierInput
}

func (r *adminUserGroupRateCaptureRepo) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	r.userID, r.rates = userID, rates
	return nil
}

func (r *adminUserGroupRateCaptureRepo) SyncGroupRateMultipliers(_ context.Context, groupID int64, entries []GroupRateMultiplierInput) error {
	r.group, r.entries = groupID, entries
	return nil
}

func TestAdminService_UpdateUser_NilGroupRateStillClears(t *testing.T) {
	repo := &adminUserGroupRateCaptureRepo{}
	svc := &adminServiceImpl{userRepo: &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42}}}, userGroupRateRepo: repo}
	_, err := svc.UpdateUser(context.Background(), 42, &UpdateUserInput{GroupRates: map[int64]*float64{10: nil}})
	require.NoError(t, err)
	require.Equal(t, int64(42), repo.userID)
	require.Contains(t, repo.rates, int64(10))
	require.Nil(t, repo.rates[10])
}

func TestAdminService_BatchInvalidRateStillRejectedWhenRateRepoUnavailable(t *testing.T) {
	svc := &adminServiceImpl{}
	err := svc.BatchSetGroupRateMultipliers(context.Background(), 10, []GroupRateMultiplierInput{{UserID: 42, RateMultiplier: -1}})
	require.Error(t, err)
}

func TestAdminService_UserGroupRateWritesRefreshBothGateways(t *testing.T) {
	ctx := context.Background()
	repo := &revisionUserGroupRateRepo{}
	svc := &adminServiceImpl{
		userRepo:          &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42}}},
		groupRepo:         &groupRepoStubForAdmin{getByID: &Group{ID: 10, Platform: PlatformOpenAI}},
		userGroupRateRepo: repo,
	}
	gateway := &GatewayService{userGroupRateResolver: newUserGroupRateResolver(repo, nil, time.Minute, nil, "test")}
	openai := &OpenAIGatewayService{userGroupRateResolver: newUserGroupRateResolver(repo, nil, time.Minute, nil, "test")}
	assertBoth := func(want float64) {
		t.Helper()
		require.Equal(t, want, gateway.ResolveUserGroupRateMultiplier(ctx, 42, 10, 1.5))
		require.Equal(t, want, openai.ResolveUserGroupRateMultiplier(ctx, 42, 10, 1.5))
	}
	assertBoth(1.5) // Cache the absence of a user override in both gateways.
	rate := 0.5
	_, err := svc.UpdateUser(ctx, 42, &UpdateUserInput{GroupRates: map[int64]*float64{10: &rate}})
	require.NoError(t, err)
	assertBoth(0.5)
	require.NoError(t, svc.BatchSetGroupRateMultipliers(ctx, 10, []GroupRateMultiplierInput{{UserID: 42, RateMultiplier: 2}}))
	assertBoth(2)
	_, err = svc.UpdateUser(ctx, 42, &UpdateUserInput{GroupRates: map[int64]*float64{10: nil}})
	require.NoError(t, err)
	assertBoth(1.5)
}

func TestAdminService_UserGroupRateValidation(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		platform                      string
		rate                          float64
		groupErr                      error
		missing, unavailable, wantErr bool
	}{
		{name: "ordinary fractional", platform: PlatformOpenAI, rate: 0.5},
		{name: "ordinary zero", platform: PlatformOpenAI, rate: 0, wantErr: true},
		{name: "ordinary positive", platform: PlatformOpenAI, rate: 0.5},
		{name: "maximum", platform: PlatformOpenAI, rate: maxUserGroupRateMultiplier},
		{name: "overflow", platform: PlatformOpenAI, rate: 1000000, wantErr: true},
		{name: "negative", platform: PlatformOpenAI, rate: -1, wantErr: true},
		{name: "nan", platform: PlatformOpenAI, rate: math.NaN(), wantErr: true},
		{name: "positive infinity", platform: PlatformOpenAI, rate: math.Inf(1), wantErr: true},
		{name: "negative infinity", platform: PlatformOpenAI, rate: math.Inf(-1), wantErr: true},
		{name: "zero missing group", rate: 0, missing: true, wantErr: true},
		{name: "zero group lookup failure", rate: 0, groupErr: errors.New("offline"), wantErr: true},
		{name: "zero unavailable group repository", rate: 0, unavailable: true, wantErr: true},
	} {
		for _, method := range []string{"UpdateUser", "BatchSetGroupRateMultipliers"} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				rateRepo := &adminUserGroupRateCaptureRepo{}
				userRepo := &rpmUserRepoStub{userRepoStub: &userRepoStub{user: &User{ID: 42}}}
				svc := &adminServiceImpl{userRepo: userRepo, userGroupRateRepo: rateRepo}
				if !tc.unavailable {
					groupRepo := &groupRepoStubForAdmin{getByID: &Group{ID: 10, Platform: tc.platform}, getErr: tc.groupErr}
					if tc.missing {
						groupRepo.getByID = nil
					}
					svc.groupRepo = groupRepo
				}
				var err error
				if method == "UpdateUser" {
					_, err = svc.UpdateUser(context.Background(), 42, &UpdateUserInput{GroupRates: map[int64]*float64{10: &tc.rate}})
				} else {
					err = svc.BatchSetGroupRateMultipliers(context.Background(), 10, []GroupRateMultiplierInput{{UserID: 42, RateMultiplier: tc.rate}})
				}
				if tc.wantErr {
					require.Error(t, err)
					require.Zero(t, rateRepo.userID)
					require.Zero(t, rateRepo.group)
					require.Nil(t, userRepo.lastUpdated)
					return
				}
				require.NoError(t, err)
				if method == "UpdateUser" {
					require.Equal(t, int64(42), rateRepo.userID)
					require.Equal(t, tc.rate, *rateRepo.rates[10])
				} else {
					require.Equal(t, int64(10), rateRepo.group)
					require.Equal(t, tc.rate, rateRepo.entries[0].RateMultiplier)
				}
			})
		}
	}
}
