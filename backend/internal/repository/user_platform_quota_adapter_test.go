//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type fakeRepoForAdapter struct {
	listByUsersIDs     []int64
	listByUsersRows    map[int64][]UserPlatformQuotaRecord
	listByUsersErr     error
	upsertCalledWith   []UserPlatformQuotaRecord
	upsertCalledUserID int64
	upsertErr          error
	resetCalledWith    [4]any // userID, platform, window, newStart
	resetErr           error
}

func (f *fakeRepoForAdapter) BulkInsertInitial(_ context.Context, _ []UserPlatformQuotaRecord) error {
	return nil
}
func (f *fakeRepoForAdapter) GetByUserPlatform(_ context.Context, _ int64, _ string) (*UserPlatformQuotaRecord, error) {
	return nil, nil
}
func (f *fakeRepoForAdapter) ListByUser(_ context.Context, _ int64) ([]UserPlatformQuotaRecord, error) {
	return nil, nil
}
func (f *fakeRepoForAdapter) IncrementUsageWithReset(_ context.Context, _ int64, _ string, _ float64, _ time.Time) error {
	return nil
}
func (f *fakeRepoForAdapter) ResetExpiredWindow(_ context.Context, userID int64, platform string, window string, newStart time.Time) error {
	f.resetCalledWith = [4]any{userID, platform, window, newStart}
	return f.resetErr
}
func (f *fakeRepoForAdapter) UpsertForUser(_ context.Context, userID int64, records []UserPlatformQuotaRecord) error {
	f.upsertCalledUserID = userID
	f.upsertCalledWith = records
	return f.upsertErr
}
func (f *fakeRepoForAdapter) BatchSnapshotUsage(_ context.Context, _ []UserPlatformQuotaSnapshot, _ time.Time) error {
	return nil
}

func TestGenericAdapter_UpsertForUser_ForwardsRecords(t *testing.T) {
	fake := &fakeRepoForAdapter{}
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.UpsertForUser(context.Background(), 42, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fake.upsertCalledUserID != 42 {
		t.Errorf("forwarded userID = %d, want 42", fake.upsertCalledUserID)
	}
}

func TestGenericAdapter_UpsertForUser_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	fake := &fakeRepoForAdapter{upsertErr: wantErr}
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.UpsertForUser(context.Background(), 1, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func TestGenericAdapter_ResetExpiredWindow_ForwardsAllParams(t *testing.T) {
	fake := &fakeRepoForAdapter{}
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if err := adapter.ResetExpiredWindow(context.Background(), 7, "openai", "weekly", now); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fake.resetCalledWith[0].(int64) != 7 ||
		fake.resetCalledWith[1].(string) != "openai" ||
		fake.resetCalledWith[2].(string) != "weekly" ||
		!fake.resetCalledWith[3].(time.Time).Equal(now) {
		t.Errorf("forwarded params mismatch: %+v", fake.resetCalledWith)
	}
}

func TestGenericAdapter_ResetExpiredWindow_PropagatesError(t *testing.T) {
	wantErr := errors.New("not found")
	fake := &fakeRepoForAdapter{resetErr: wantErr}
	adapter := NewUserPlatformQuotaServiceAdapter(fake)

	err := adapter.ResetExpiredWindow(context.Background(), 1, "a", "daily", time.Now())
	if !errors.Is(err, wantErr) {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}

func (s *fakeRepoForAdapter) ListByUsers(_ context.Context, ids []int64) (map[int64][]UserPlatformQuotaRecord, error) {
	s.listByUsersIDs = append([]int64(nil), ids...)
	return s.listByUsersRows, s.listByUsersErr
}

func TestGenericAdapter_ListByUsersPreservesGrokRefusalUsage(t *testing.T) {
	now := time.Now()
	quota := service.GrokRefusalQuotaUsage{}
	quota.Add(0.25, now, nil)
	raw, err := json.Marshal(quota)
	require.NoError(t, err)
	var durable map[string]any
	require.NoError(t, json.Unmarshal(raw, &durable))
	limit := 10.0
	fake := &fakeRepoForAdapter{listByUsersRows: map[int64][]UserPlatformQuotaRecord{
		7: {{UserID: 7, Platform: service.PlatformGrok, DailyLimitUSD: &limit,
			DailyUsageUSD: 1, WeeklyUsageUSD: 1, MonthlyUsageUSD: 1,
			DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now,
			GrokRefusalUsage: durable}},
		8: {{UserID: 8, Platform: service.PlatformOpenAI, DailyUsageUSD: 2}},
	}}
	adapter := NewUserPlatformQuotaServiceAdapter(fake)
	rows, err := adapter.ListByUsers(context.Background(), []int64{7, 8})
	require.NoError(t, err)
	require.Equal(t, []int64{7, 8}, fake.listByUsersIDs)
	require.InDelta(t, 1.25, rows[7][0].DailyUsageUSD, 1e-8)
	require.InDelta(t, 1.25, rows[7][0].WeeklyUsageUSD, 1e-8)
	require.InDelta(t, 1.25, rows[7][0].MonthlyUsageUSD, 1e-8)
	require.Equal(t, 2.0, rows[8][0].DailyUsageUSD)
	require.Equal(t, 1.0, fake.listByUsersRows[7][0].DailyUsageUSD, "must not mutate the base snapshot")
	fake.listByUsersErr = context.DeadlineExceeded
	rows, err = adapter.ListByUsers(context.Background(), []int64{7})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, rows)
}
