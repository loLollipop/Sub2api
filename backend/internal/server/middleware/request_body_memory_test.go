package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRequestBodyMemoryRejectsOverloadAndReleasesAfterHandler(t *testing.T) {
	router := gin.New()
	held := make(chan struct{})
	release := make(chan struct{})
	router.POST("/v1/responses", RequestBodyLimit(2048, pkghttputil.NewBodyMemoryBudget(1024)), func(c *gin.Context) {
		_, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if RejectRequestBodyMemoryOverload(c, err) {
			return
		}
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if c.Query("hold") == "1" {
			close(held)
			<-release
		}
		c.Status(http.StatusNoContent)
	})
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", 700))))
		return w
	}
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { firstDone <- request("/v1/responses?hold=1") }()
	releaseHeld := sync.OnceFunc(func() { close(release) })
	defer releaseHeld()
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not reach handler")
	}
	w := request("/v1/responses")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Equal(t, "1", w.Header().Get("Retry-After"))
	releaseHeld()
	select {
	case first := <-firstDone:
		require.Equal(t, http.StatusNoContent, first.Code)
	case <-time.After(5 * time.Second):
		t.Fatal("held request did not finish")
	}
	require.Equal(t, http.StatusNoContent, request("/v1/responses").Code)
}

func TestRejectRequestBodyReadErrorCancellation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cancelled bool
		committed bool
	}{
		{name: "cancelled", cancelled: true},
		{name: "cancelled_after_commit", cancelled: true, committed: true},
		{name: "ordinary_read_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			if tc.committed {
				_, err := c.Writer.WriteString("already committed")
				require.NoError(t, err)
			}
			bodyBefore := recorder.Body.String()
			require.Equal(t, tc.cancelled, RejectRequestBodyReadError(c, io.ErrUnexpectedEOF))
			require.Equal(t, tc.cancelled, c.IsAborted())
			status := http.StatusOK
			if tc.cancelled && !tc.committed {
				status = 499
			}
			require.Equal(t, status, c.Writer.Status())
			require.Equal(t, bodyBefore, recorder.Body.String())
		})
	}
}

func TestRequestBodyMemoryOverloadProtocol(t *testing.T) {
	for _, tc := range []struct {
		path, field, want string
	}{
		{"/v1/responses", "error.type", "server_error"},
		{"/responses", "error.code", "request_body_memory_exhausted"},
		{"/backend-api/codex/responses", "error.type", "server_error"},
		{"/messages", "type", "error"},
		{"/antigravity/v1/messages", "error.type", "api_error"},
		{"/v1beta/models/gemini:generateContent", "error.status", "UNAVAILABLE"},
		{"/antigravity/v1beta/models/gemini:generateContent", "error.code", "503"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			router := gin.New()
			router.POST(tc.path, RequestBodyLimit(1024, pkghttputil.NewBodyMemoryBudget(8)), func(c *gin.Context) {
				_, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
				require.ErrorIs(t, err, pkghttputil.ErrRequestBodyMemoryBudget)
				require.True(t, RejectRequestBodyReadError(c, err))
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"model":"example"}`)))
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "1", rec.Header().Get("Retry-After"))
			require.Equal(t, tc.want, gjson.Get(rec.Body.String(), tc.field).String())
		})
	}
}

func TestGroupModelAllowlistNormalizationMemoryBudget(t *testing.T) {
	router := gin.New()
	router.Use(RequestBodyLimit(2048, pkghttputil.NewBodyMemoryBudget(700)), func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
			ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"model"}},
		}})
		c.Next()
	}, GroupModelAllowlist(2048))
	router.POST("/v1/responses", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	// Raw body fits, but original + normalized replacement exceed the shared
	// budget. This must be 503 before model validation, not invalid JSON/413.
	rec := doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"model","input":"`+strings.Repeat("\x00", 100)+`"}`)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	// All failed allocations were refunded before the next request.
	rec = doJSON(t, router, http.MethodPost, "/v1/responses", `{"model":"model"}`)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

func TestRequestBodyMemoryReleasesAfterAbortAndPanic(t *testing.T) {
	budget := pkghttputil.NewBodyMemoryBudget(1024)
	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(http.StatusInternalServerError) }))
	router.POST("/body", RequestBodyLimit(2048, budget), func(c *gin.Context) {
		_, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if RejectRequestBodyReadError(c, err) {
			return
		}
		require.NoError(t, err)
		switch c.Query("exit") {
		case "abort":
			c.AbortWithStatus(http.StatusForbidden)
		case "panic":
			panic("test handler failure")
		default:
			c.Status(http.StatusNoContent)
		}
	})
	for _, tc := range []struct {
		mode   string
		status int
	}{
		{"abort", http.StatusForbidden}, {"panic", http.StatusInternalServerError}, {"ok", http.StatusNoContent},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/body?exit="+tc.mode, strings.NewReader(strings.Repeat("x", 700))))
		require.Equal(t, tc.status, rec.Code)
	}
}
