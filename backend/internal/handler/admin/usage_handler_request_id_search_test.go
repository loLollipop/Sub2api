package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminUsageRequestIDValidationIncludesInflightOnly(t *testing.T) {
	for _, inflight := range []string{"false", "true"} {
		for _, tc := range []struct {
			input, start, end string
			status            int
		}{
			{"ab", "", "", 400}, {"client:a", "", "", 400}, {"%_", "", "", 400},
			{"=", "2026-10-13", "2026-10-13", 400}, {"^", "2026-10-13", "2026-10-13", 400},
			{"ab", "2026-10-13", "2026-10-14", 400},
			{"ab", "2026-10-14", "2026-10-12", 400},
			{"ab", "2026-10-13T00:00:00", "2026-10-14T00:00:00", 200},
			{"ab", "2026-10-13T00:00:00", "2026-10-14T00:00:01", 400},
			{"ab", "2026-10-13", "2026-10-13", 200},
			{"=a", "", "", 200}, {"^abc", "", "", 200}, {"abc", "", "", 200},
		} {
			t.Run(inflight+tc.input+tc.start+tc.end, func(t *testing.T) {
				repo := &adminUsageRepoCapture{}
				router := newAdminUsageRequestTypeTestRouter(repo)
				q := url.Values{"inflight_only": {inflight}, "request_id": {tc.input}, "start_date": {tc.start}, "end_date": {tc.end}, "timezone": {"UTC"}}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/usage?"+q.Encode(), nil))
				require.Equal(t, tc.status, w.Code, w.Body.String())
				if tc.status == 400 {
					require.Empty(t, repo.listFilters.RequestID)
				}
			})
		}
	}
}
