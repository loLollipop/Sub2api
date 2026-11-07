//go:build unit

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAPIKeyLookupClientGoneRequiresActualCancellation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requestState string
		err          error
		gone         bool
	}{
		{"cancelled", "cancel", fmt.Errorf("wrapped: %w", context.Canceled), true},
		{"server error after cancel", "cancel", errors.New("db contains sk-secret 10.0.0.1:5432"), false},
		{"lookup cancelled without client cancel", "", context.Canceled, false},
		{"server timeout", "deadline", context.DeadlineExceeded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, entries := observer.New(zap.WarnLevel)
			ctx := logger.IntoContext(context.Background(), zap.New(core))
			if tc.requestState == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if tc.requestState == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			require.Equal(t, tc.gone, abortIfAPIKeyLookupClientGone(c, tc.err))
			if tc.gone {
				require.Equal(t, 499, rec.Code)
				require.Zero(t, entries.Len())
			} else {
				require.Equal(t, 1, entries.Len())
				require.NotContains(t, fmt.Sprint(entries.All()), "sk-secret")
				require.NotContains(t, fmt.Sprint(entries.All()), "10.0.0.1")
			}
		})
	}
}
