package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromInflightRoutingNames(t *testing.T) {
	now := time.Now()
	row := service.UsageInflightSnapshot{
		RequestID: "routing-test", UserID: 1, APIKeyID: 2,
		GroupID: 3, GroupName: "Selected Group", AccountID: 4, AccountName: "Private Account",
		StartedAt: now.Add(-time.Second),
	}
	user := UsageLogFromInflight(row, now)
	require.NotNil(t, user.Group)
	require.Equal(t, int64(3), user.Group.ID)
	require.Equal(t, "Selected Group", user.Group.Name)
	encoded, err := json.Marshal(user)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "Private Account")
	admin := AdminUsageLogFromInflight(row, now)
	require.NotNil(t, admin.Account)
	require.Equal(t, int64(4), admin.Account.ID)
	require.Equal(t, "Private Account", admin.Account.Name)
	require.Equal(t, "Selected Group", admin.Group.Name)
	row.AccountID, row.GroupID = 0, 0
	row.AccountName, row.GroupName = "stale account", "stale group"
	admin = AdminUsageLogFromInflight(row, now)
	require.Nil(t, admin.Account)
	require.Nil(t, admin.Group)
}
