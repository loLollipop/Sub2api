//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelUltrafastPricingSQLRoundTrip(t *testing.T) {
	for _, multiplier := range []*float64{nil, floatPointer(1), floatPointer(1.25)} {
		card := service.ChannelModelPricing{ID: 11, ChannelID: 7, Platform: service.PlatformOpenAI, Models: []string{"gpt-6.2-sol"}, UltrafastMultiplier: multiplier, CacheWrite12hPrice: floatPointer(0)}
		row := func() *sqlmock.Rows {
			return sqlmock.NewRows(channelModelPricingTimePricingColumns).AddRow(
				int64(11), int64(7), service.PlatformOpenAI, `["gpt-6.2-sol"]`, service.BillingModeToken,
				nil, nil, nil, nil, 0.0, nil, nil, nil, multiplier, nil, nil, nil, nil, nil, time.Time{}, time.Time{},
			)
		}
		for _, operation := range []string{"list", "batch", "create", "update", "replace"} {
			t.Run(operation, func(t *testing.T) {
				repo, mock := newChannelModelPricingTimePricingRepo(t)
				ctx := context.Background()
				switch operation {
				case "list", "batch":
					mock.ExpectQuery(`(?s)SELECT .*ultrafast_multiplier.*FROM channel_model_pricing`).WithArgs(sqlmock.AnyArg()).WillReturnRows(row())
					expectEmptyModelPricingIntervals(mock)
					var stored []service.ChannelModelPricing
					if operation == "list" {
						var err error
						stored, err = repo.ListModelPricing(ctx, 7)
						require.NoError(t, err)
					} else {
						batch, err := repo.batchLoadModelPricing(ctx, []int64{7})
						require.NoError(t, err)
						stored = batch[7]
					}
					require.Len(t, stored, 1)
					require.Equal(t, multiplier, stored[0].UltrafastMultiplier)
					require.NotNil(t, stored[0].CacheWrite12hPrice)
					require.Zero(t, *stored[0].CacheWrite12hPrice)
				case "create", "replace":
					args := []driver.Value{int64(7), service.PlatformOpenAI, []byte(`["gpt-6.2-sol"]`), service.BillingModeToken,
						nil, nil, nil, nil, 0.0, nil, nil, nil, multiplier, nil, nil, nil, nil, nil}
					if operation == "replace" {
						mock.ExpectBegin()
						mock.ExpectExec(`DELETE FROM channel_model_pricing WHERE channel_id`).WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
					}
					mock.ExpectQuery(`INSERT INTO channel_model_pricing .*ultrafast_multiplier`).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(11), time.Time{}, time.Time{}))
					if operation == "replace" {
						mock.ExpectCommit()
						require.NoError(t, repo.ReplaceModelPricing(ctx, 7, []service.ChannelModelPricing{card}))
					} else {
						require.NoError(t, repo.CreateModelPricing(ctx, &card))
					}
				case "update":
					mock.ExpectExec(`UPDATE channel_model_pricing`).WithArgs(
						[]byte(`["gpt-6.2-sol"]`), service.BillingModeToken,
						nil, nil, nil, nil, 0.0, nil, nil, nil, multiplier, nil, nil, nil, nil, nil, service.PlatformOpenAI, int64(11),
					).WillReturnResult(sqlmock.NewResult(0, 1))
					require.NoError(t, repo.UpdateModelPricing(ctx, &card))
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func floatPointer(v float64) *float64 { return &v }
