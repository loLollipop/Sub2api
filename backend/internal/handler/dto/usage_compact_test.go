package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogCompactDTOSeparatesSummariesAndAdminIdentity(t *testing.T) {
	deleted := time.Now().UTC()
	log := &service.UsageLog{
		AssociationsCompact: true, AccountNameOnly: true,
		User:    &service.User{ID: 1, Email: "deleted@example.test", DeletedAt: &deleted},
		APIKey:  &service.APIKey{ID: 2, Name: "key", Key: "synthetic-secret"},
		Account: &service.Account{ID: 3, Name: "account", Credentials: map[string]any{"token": "synthetic-credential"}},
		Group:   &service.Group{ID: 4, Name: "group"},
	}
	admin := UsageLogFromServiceAdmin(log)
	require.Nil(t, admin.User)
	require.Nil(t, admin.APIKey)
	require.Nil(t, admin.Group)
	require.Nil(t, admin.Account)
	require.Equal(t, "deleted@example.test", admin.UserSummary.Email)
	require.NotNil(t, admin.UserSummary.DeletedAt)
	require.Equal(t, "key", admin.APIKeySummary.Name)
	require.Equal(t, "group", admin.GroupSummary.Name)
	require.Equal(t, "account", admin.AccountSummary.Name)
	encoded, err := json.Marshal(admin)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "synthetic-secret")
	require.NotContains(t, string(encoded), "synthetic-credential")
	userJSON, err := json.Marshal(UsageLogFromService(log))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "account_summary")
	log.AssociationsCompact = false
	legacy := UsageLogFromServiceAdmin(log)
	require.Equal(t, "synthetic-secret", legacy.APIKey.Key)
	require.NotNil(t, legacy.User)
	require.NotNil(t, legacy.Group)
	require.NotNil(t, legacy.Account)
	require.Nil(t, legacy.AccountSummary)
}
