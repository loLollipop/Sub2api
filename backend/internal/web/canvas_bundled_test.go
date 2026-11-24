package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInfiniteCanvasHandler_BundledAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INFINITE_CANVAS_STATIC_DIR", t.TempDir())
	t.Setenv("INFINITE_CANVAS_UPSTREAM", "")
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>image canvas</html>")},
		"assets/app.js": {Data: []byte("export const canvas = true;")},
	}
	router := gin.New()
	router.Use(newInfiniteCanvasHandler(assets))
	for _, test := range []struct {
		path, method      string
		status            int
		body, contentType string
	}{
		{"/canvas/", "GET", 200, "image canvas", "text/html"},
		{"/canvas/canvas/project-1", "GET", 200, "image canvas", "text/html"},
		{"/canvas/", "HEAD", 200, "", "text/html"},
		{"/canvas/assets/app.js", "GET", 200, "export const", "javascript"},
		{"/canvas/assets/missing.js", "GET", 404, "", ""},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			require.Equal(t, test.status, response.Code)
			require.Contains(t, response.Body.String(), test.body)
			require.Contains(t, response.Header().Get("Content-Type"), test.contentType)
			if test.contentType == "text/html" {
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			}
			require.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
			if test.method == http.MethodHead {
				require.Empty(t, response.Body.String())
			}
		})
	}
}

func TestCanvasLocalImageReadPolicy(t *testing.T) {
	header := http.Header{}
	header.Add("Content-Security-Policy", "default-src 'self'; connect-src 'self' https://images.example; script-src 'self'")
	header.Add("Content-Security-Policy", "default-src 'self'; object-src 'none'")
	allowCanvasLocalImageReads(header)
	values := header.Values("Content-Security-Policy")
	require.Len(t, values, 2)
	require.Contains(t, values[0], "connect-src 'self' https://images.example blob: data:")
	require.Contains(t, values[0], "script-src 'self'")
	require.Contains(t, values[1], "connect-src 'self' blob: data:")
	require.Contains(t, values[1], "object-src 'none'")
	allowCanvasLocalImageReads(header)
	require.Equal(t, values, header.Values("Content-Security-Policy"))
}
