//go:build unit

package handler

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/stretchr/testify/require"
)

func TestGrokRelayCompletedNativeHandlerSettlesPersistentJobOnce(t *testing.T) {
	f := newDurableMediaHandlerFixture(t)
	require.Equal(t, http.StatusOK, f.request(0, http.MethodPost, "create", nil).Code)
	f.reorderKey()
	f.upstream.statusBody = `{"status":"completed","data":[{"url":"https://vidgen.x.ai/test.mp4"}],"seconds":6}`
	for instance := range 2 {
		response := f.request(instance, http.MethodGet, "status", nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"status":"completed"`)
		require.Contains(t, response.Body.String(), `https://vidgen.x.ai/test.mp4`)
	}
	job, balance, applied, failed, direct := f.repo.snapshotByTask(f.taskID)
	require.Equal(t, service.MediaBillingDone, job.Status)
	require.Zero(t, balance)
	require.Equal(t, 1, applied)
	require.Zero(t, failed)
	require.Zero(t, direct)
	require.NotNil(t, job.Intent)
	require.Equal(t, int64(17), *job.Intent.Usage.GroupID)
	require.InDelta(t, f.initial, job.Intent.Command.BalanceCost, 1e-8)
	logs, _ := f.usage.snapshot()
	require.Len(t, logs, 1)
}

func TestGrokRelayCompletedWithoutAssetRetainsPersistentHold(t *testing.T) {
	for index, body := range []string{`{"status":"completed"}`, `{"status":"completed","data":[{"url":null}]}`, `{"status":"completed","data":[{"url":" "}]}`, `{"status":"future"}`} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			f := newDurableMediaHandlerFixture(t)
			require.Equal(t, http.StatusOK, f.request(0, http.MethodPost, "create", nil).Code)
			f.upstream.statusBody = body
			response := f.request(1, http.MethodGet, "status", nil)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			job, balance, applied, failed, direct := f.repo.snapshotByTask(f.taskID)
			require.Equal(t, service.MediaBillingSubmitted, job.Status)
			require.Zero(t, balance)
			require.InDelta(t, f.initial, job.ReservedAmount, 1e-8)
			require.Zero(t, applied)
			require.Zero(t, failed)
			require.Zero(t, direct)
		})
	}
}
