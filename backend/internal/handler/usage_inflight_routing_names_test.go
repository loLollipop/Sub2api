//go:build unit

package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetOpsSelectedAccountInflightRoutingNames(t *testing.T) {
	progress := service.NewUsageInflightProgress(service.UsageInflightSnapshot{})
	ctx := service.WithUsageInflightProgress(context.Background(), progress)
	ctx = context.WithValue(ctx, ctxkey.Group, &service.Group{ID: 3, Name: " First Group "})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
	setOpsSelectedAccount(c, 10, "openai", " First Account ")
	require.Equal(t, int64(10), progress.Snapshot().AccountID)
	require.Equal(t, "First Account", progress.Snapshot().AccountName)
	require.Equal(t, "First Group", progress.Snapshot().GroupName)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.Group, &service.Group{ID: 4, Name: "Fallback Group"}))
	setOpsSelectedAccount(c, 11, "openai", "Fallback Account")
	require.Equal(t, int64(4), progress.Snapshot().GroupID)
	require.Equal(t, "Fallback Group", progress.Snapshot().GroupName)
	require.Equal(t, "Fallback Account", progress.Snapshot().AccountName)
	setOpsSelectedAccount(c, 12)
	require.Equal(t, int64(12), progress.Snapshot().AccountID)
	require.Empty(t, progress.Snapshot().AccountName)
}
