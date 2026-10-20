package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestV226ResponsesAuditLatestRealUser(t *testing.T) {
	for _, tail := range []string{
		`{"type":"function_call_output","output":"tool"}`,
		`{"type":"reasoning","text":"private reasoning"}`,
		`{"type":"message","content":"untrusted missing role"}`,
		`{"type":"message","role":"unknown","content":"unknown role"}`,
		`{"type":"message","role":"assistant","content":"assistant"}`,
	} {
		body := []byte(`{"input":[{"type":"message","role":"user","content":"latest user"},` + tail + `]}`)
		require.Equal(t, "latest user", ExtractContentModerationLastUserTurn(ContentModerationProtocolOpenAIResponses, body))
	}
	body := []byte(`{"input":[{"type":"message","role":"user","content":"old user"},{"type":"function_call_output","output":"tool"}]}`)
	require.Empty(t, ExtractContentModerationInput(ContentModerationProtocolOpenAIResponses, body).Text)
}

func TestV226AnthropicStreamHTTPErrorStatus(t *testing.T) {
	for typ, want := range map[string]int{"invalid_request_error": 400, "not_found_error": 404, "authentication_error": 401, "permission_error": 403, "overloaded_error": 529} {
		status, _, _ := anthropicStreamErrorSemantics(`{"type":"error","error":{"type":"` + typ + `","message":"failed"}}`)
		require.Equal(t, want, status, typ)
	}
}

func TestV226MessagesKeepaliveGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, time.Hour)
	defer stop()
	value, _ := c.Get(openAICompactSSEKeepaliveKey)
	k, ok := value.(*openAICompactSSEKeepalive)
	require.True(t, ok)
	require.True(t, k.beat())
	n, err, handled := WriteOpenAIStreamSSEHeartbeat(c, []byte(": slot\n\n"))
	require.NoError(t, err)
	require.True(t, handled)
	require.Zero(t, n)
	require.False(t, c.Writer.Written())
	ArmOpenAIStreamSSEKeepalive(c)
	require.False(t, IsOpenAIStreamSSEKeepaliveCommitted(c), "upstream ready does not commit the response")
	require.True(t, k.beat())
	require.True(t, IsOpenAIStreamSSEKeepaliveCommitted(c))
}

func TestV226MessagesGateTimerAndErrorStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, 529} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		stop := StartOpenAIStreamSSEKeepaliveUntilUpstream(c, time.Millisecond)
		time.Sleep(10 * time.Millisecond)
		require.False(t, c.Writer.Written())
		c.JSON(status, gin.H{"error": "upstream rejected"})
		require.Equal(t, status, c.Writer.Status())
		stop()
	}
}

func TestV226CyberExcerptSnapshotPrivacy(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,c2VjcmV0"}]}]}`)
	excerpt := CyberPolicyInputExcerpt(ContentModerationProtocolOpenAIResponses, body)
	require.Equal(t, "[images: 1]", excerpt)
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, "[images: 1]", excerpt)
	for _, input := range []string{
		`{"type":"message","content":"suspicious"}`,
		`{"type":"message","role":"alien","content":"suspicious"}`,
		`{"type":"reasoning","role":"user","text":"reasoning"}`,
		`{"type":"function_call_output","role":"user","text":"tool"}`,
	} {
		require.Empty(t, CyberPolicyInputExcerpt(ContentModerationProtocolOpenAIResponses, []byte(`{"input":[`+input+`]}`)))
	}
}
