package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAINonStreamingProtocolErrorCommitsTerminalBody(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	err := (&OpenAIGatewayService{}).writeOpenAINonStreamingProtocolError(resp, c, "provider rejected request")
	require.Error(t, err)
	require.True(t, IsResponseCommitted(c))
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.True(t, json.Valid(w.Body.Bytes()), "terminal response must remain one JSON document")
}
