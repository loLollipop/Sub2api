package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// GetLeaderboardUsage reports retained raw usage only. Existing dashboard
// rollups cannot prove completeness after late inserts or reconstruct all
// timezone boundaries, so mixing them would silently change the ranking.
func (r *usageLogRepository) GetLeaderboardUsage(ctx context.Context, start, end time.Time, granularity string) ([]service.LeaderboardUsageBucket, error) {
	result := make([]service.LeaderboardUsageBucket, 0)
	if !end.After(start) {
		return result, nil
	}
	if end.Sub(start) > 367*24*time.Hour {
		return nil, fmt.Errorf("leaderboard range exceeds one year")
	}
	if granularity != "hour" && granularity != "day" && granularity != "month" {
		return nil, fmt.Errorf("invalid leaderboard granularity")
	}
	query := `
        SELECT CASE WHEN $3 = 'hour' THEN date_bin(interval '1 hour', created_at, $1::timestamptz)
               ELSE date_trunc($3, created_at AT TIME ZONE $4) AT TIME ZONE $4 END AS date,
               COALESCE(NULLIF(TRIM(requested_model), ''), model) AS model,
               COALESCE(SUM(input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens), 0)::bigint,
               COUNT(*)::bigint
        FROM usage_logs
        WHERE created_at >= $1 AND created_at < $2
        GROUP BY 1, 2 ORDER BY 1, 2`
	rows, err := r.sql.QueryContext(ctx, query, start.UTC(), end.UTC(), granularity, timezone.Location().String())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row service.LeaderboardUsageBucket
		if err := rows.Scan(&row.Date, &row.Model, &row.TotalTokens, &row.Requests); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
