package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAsyncImageTaskRead(t *testing.T) {
	require.True(t, isAsyncImageTaskRead(http.MethodGet, "/v1/images/tasks/imgtask_123"))
	require.True(t, isAsyncImageTaskRead(http.MethodGet, "/images/tasks/imgtask_123"))
	require.False(t, isAsyncImageTaskRead(http.MethodPost, "/v1/images/tasks/imgtask_123"))
	require.False(t, isAsyncImageTaskRead(http.MethodGet, "/v1/images/generations"))
}

func TestIsGrokVideoTaskRead(t *testing.T) {
	for _, prefix := range []string{"", "/v1"} {
		for _, alias := range []string{"", "/generations", "/edits", "/extensions"} {
			for _, suffix := range []string{"", "/content"} {
				path := prefix + "/videos" + alias + "/task" + suffix
				require.True(t, isGrokVideoTaskRead(http.MethodGet, path), path)
				require.False(t, isGrokVideoTaskRead(http.MethodPost, path), path)
			}
		}
	}
	for _, path := range []string{"/v1/videos/generations", "/v1/videos/edits", "/v1/videos/", "/v1/videos/task/other", "/v1/videos/task/content/extra", "/v1videos/task", "/v1/images/generations"} {
		require.False(t, isGrokVideoTaskRead(http.MethodGet, path), path)
	}
}
