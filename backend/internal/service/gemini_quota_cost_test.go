package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestGeminiAggregateUsageUsesAccountCost(t *testing.T) {
	totals := geminiAggregateUsage([]usagestats.ModelStat{
		{Model: "gemini-3.7-flash", Requests: 2, TotalTokens: 20, ActualCost: 5, AccountCost: 1.5},
		{Model: "gemini-4-argon", Requests: 1, TotalTokens: 10, ActualCost: 8, AccountCost: 3},
		// A free account cost must stay zero even when the customer was charged.
		{Model: "gemini-3.6-flash", Requests: 1, TotalTokens: 7, ActualCost: .15, AccountCost: 0},
		{Model: "gemini-4-argon", Requests: 1, TotalTokens: 9, ActualCost: .15, AccountCost: 0},
	})
	require.Equal(t, int64(3), totals.FlashRequests)
	require.Equal(t, int64(27), totals.FlashTokens)
	require.Equal(t, 1.5, totals.FlashCost)
	require.Equal(t, int64(2), totals.ProRequests)
	require.Equal(t, int64(19), totals.ProTokens)
	require.Equal(t, 3.0, totals.ProCost)
}
