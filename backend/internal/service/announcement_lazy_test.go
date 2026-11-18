package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lazyAnnouncementRepo struct {
	AnnouncementRepository
	items                    []Announcement
	metadataReads, bodyLists int
	bodyIDs                  []int64
}

func (r *lazyAnnouncementRepo) ListActiveMetadata(context.Context, time.Time) ([]Announcement, error) {
	r.metadataReads++
	items := append([]Announcement(nil), r.items...)
	for i := range items {
		items[i].Content = ""
	}
	return items, nil
}
func (r *lazyAnnouncementRepo) ListActive(context.Context, time.Time) ([]Announcement, error) {
	r.bodyLists++
	return r.items, nil
}
func (r *lazyAnnouncementRepo) GetByID(_ context.Context, id int64) (*Announcement, error) {
	r.bodyIDs = append(r.bodyIDs, id)
	for _, a := range r.items {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, ErrAnnouncementNotFound
}

type lazyAnnouncementUserRepo struct{ UserRepository }

func (lazyAnnouncementUserRepo) GetByID(context.Context, int64) (*User, error) {
	return &User{ID: 1, Balance: 2}, nil
}

type lazyAnnouncementSubRepo struct{ UserSubscriptionRepository }

func (lazyAnnouncementSubRepo) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

type lazyAnnouncementReadRepo struct {
	AnnouncementReadRepository
	reads  map[int64]time.Time
	marked []int64
}

func (r *lazyAnnouncementReadRepo) GetReadMapByUser(_ context.Context, _ int64, ids []int64) (map[int64]time.Time, error) {
	out := map[int64]time.Time{}
	for _, id := range ids {
		if at, ok := r.reads[id]; ok {
			out[id] = at
		}
	}
	return out, nil
}
func (r *lazyAnnouncementReadRepo) MarkRead(_ context.Context, id, _ int64, at time.Time) error {
	r.marked = append(r.marked, id)
	r.reads[id] = at
	return nil
}

func TestAnnouncementLazySummaryAndPopupBodyBound(t *testing.T) {
	repo := &lazyAnnouncementRepo{}
	for id := int64(10); id > 0; id-- {
		repo.items = append(repo.items, Announcement{ID: id, Title: "title", Content: "body", Status: AnnouncementStatusActive, NotifyMode: AnnouncementNotifyModePopup})
	}
	repo.items[0].Status = AnnouncementStatusDraft
	repo.items[1].Targeting = AnnouncementTargeting{AnyOf: []AnnouncementConditionGroup{{AllOf: []AnnouncementCondition{{Type: AnnouncementConditionTypeBalance, Operator: AnnouncementOperatorGT, Value: 5}}}}}
	reads := &lazyAnnouncementReadRepo{reads: map[int64]time.Time{8: time.Now()}}
	svc := NewAnnouncementService(repo, reads, lazyAnnouncementUserRepo{}, lazyAnnouncementSubRepo{})
	total, unread, err := svc.SummaryForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 8, total)
	require.Equal(t, 7, unread)
	require.Equal(t, 0, repo.bodyLists)
	require.Empty(t, repo.bodyIDs)
	popups, err := svc.PopupsForUser(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, popups, 5)
	require.Equal(t, []int64{7, 6, 5, 4, 3}, repo.bodyIDs)
	require.Equal(t, 0, repo.bodyLists)
	for _, popup := range popups {
		require.Equal(t, "body", popup.Announcement.Content)
		require.Nil(t, popup.ReadAt)
	}
}

func TestAnnouncementLazyDetailAndReadUseSameVisibility(t *testing.T) {
	repo := &lazyAnnouncementRepo{items: []Announcement{{ID: 1, Content: "visible", Status: AnnouncementStatusActive}, {ID: 2, Content: "hidden", Status: AnnouncementStatusDraft}}}
	reads := &lazyAnnouncementReadRepo{reads: map[int64]time.Time{}}
	svc := NewAnnouncementService(repo, reads, lazyAnnouncementUserRepo{}, lazyAnnouncementSubRepo{})
	item, err := svc.GetForUser(context.Background(), 1, 1)
	require.NoError(t, err)
	require.Equal(t, "visible", item.Announcement.Content)
	require.NoError(t, svc.MarkRead(context.Background(), 1, 1))
	item, err = svc.GetForUser(context.Background(), 1, 1)
	require.NoError(t, err)
	require.NotNil(t, item.ReadAt)
	for _, id := range []int64{2, 3} {
		_, err = svc.GetForUser(context.Background(), 1, id)
		require.ErrorIs(t, err, ErrAnnouncementNotFound)
		require.ErrorIs(t, svc.MarkRead(context.Background(), 1, id), ErrAnnouncementNotFound)
	}
	require.Equal(t, []int64{1}, reads.marked)
}
