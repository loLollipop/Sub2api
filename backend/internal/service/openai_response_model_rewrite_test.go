package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A lifecycle timestamp must survive rewriting response.model in both processors.
func TestMappedResponseModelForwardingPreservesCreatedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "passthrough"}[passthrough], func(t *testing.T) {
			payload := `{"type":"response.completed","response":{"id":"resp_1","created_at":1700000000,"model":"upstream-model","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`
			body := "data: " + payload + "\n\ndata: [DONE]\n\n"
			want := strings.Replace(payload, `"model":"upstream-model"`, `"model":"public-model"`, 1)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
			svc := &OpenAIGatewayService{}
			account := &Account{ID: 1}
			var err error
			if passthrough {
				_, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "public-model", "upstream-model")
			} else {
				_, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "public-model", "upstream-model")
			}
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), want)
			require.Equal(t, "upstream-model", observedUpstreamResponseModel(c))
		})
	}
}
