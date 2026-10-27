package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientErrorPrivacyOpenAIStreamConflictingEnvelopeAtWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const success = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Source https://public.example\"}\n\n"
	for _, fields := range []string{
		`"type":"response.output_text.delta","type":"error","error":{"api_key":"synthetic-private-value"}`,
		`"type":"response.output_text.delta","error":{},"error":null,"api_key":"synthetic-private-value"`,
		`"type":"response.output_text.delta","status":"failed","status":"completed","api_key":"synthetic-private-value"`,
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		failure := `{` + fields + `,"message":"https://private-provider.example"}`
		wire := success + "data: " + failure + "\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_safe\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
		_, _ = (&OpenAIGatewayService{cfg: &config.Config{}}).handleStreamingResponsePassthrough(context.Background(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, time.Now(), "fixture", "fixture")
		require.Contains(t, rec.Body.String(), "https://public.example")
		require.NotContains(t, rec.Body.String(), "private-provider")
		require.NotContains(t, rec.Body.String(), "synthetic-private-value")
	}
}

func TestClientErrorPrivacyOpenAIUsageAlwaysUsesOriginalStreamBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const first = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Source https://public.example\"}\n\n"
	for _, passthrough := range []bool{false, true} {
		for _, message := range []string{"safe diagnostic", "https://private-provider.example"} {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			payload := `{"type":"response.output_text.delta","type":"error","message":"` + message + `","usage":{"input_tokens":11,"output_tokens":3},"usage":{"input_tokens":999,"output_tokens":999}}`
			wire := first + "data: " + payload + "\n\ndata: [DONE]\n\n"
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, toolCorrector: NewCodexToolCorrector()}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
			var usage *OpenAIUsage
			if passthrough {
				result, err := svc.handleStreamingResponsePassthrough(t.Context(), resp, c, account, time.Now(), "fixture", "fixture")
				require.NoError(t, err)
				require.NotNil(t, result)
				usage = result.usage
			} else {
				result, err := svc.handleStreamingResponse(t.Context(), resp, c, account, time.Now(), "fixture", "fixture")
				require.NoError(t, err)
				require.NotNil(t, result)
				usage = result.usage
			}
			require.Equal(t, 11, usage.InputTokens, "passthrough=%t, diagnostic=%q", passthrough, message)
			require.Equal(t, 3, usage.OutputTokens, "usage must not come from a redactor's duplicate-key projection")
			require.NotContains(t, rec.Body.String(), "private-provider")
			require.Contains(t, rec.Body.String(), "https://public.example")
		}
	}
}

func TestClientErrorPrivacyImageAndMediaJSONWrites(t *testing.T) {
	for _, target := range []string{"image", "media"} {
		for _, body := range []string{
			`{"data":[{"url":"https://public.example/image.png"}]}`,
			`{"error":{},"error":null,"message":"https://private-provider.example","api_key":"synthetic-private-value"}`,
		} {
			t.Run(target+"/"+body, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				original := []byte(body)
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(original))}
				if target == "image" {
					_, _, _, err := (&OpenAIGatewayService{cfg: &config.Config{}}).handleOpenAIImagesNonStreamingResponse(t.Context(), resp, c, &Account{ID: 1}, &OpenAIImagesRequest{})
					require.NoError(t, err)
				} else {
					writeGrokMediaResponse(c, resp, original, nil)
				}
				require.Equal(t, body, string(original))
				if strings.Contains(body, "public.example") {
					require.Equal(t, body, rec.Body.String())
				} else {
					require.NotContains(t, rec.Body.String(), "private-provider")
					require.NotContains(t, rec.Body.String(), "synthetic-private-value")
				}
			})
		}
	}
}

func TestClientErrorPrivacyMediaDownloadHeadersKeepBinaryContent(t *testing.T) {
	for _, contentType := range []string{
		`video/mp4; upstream="https://private-provider.example"`, "video/mp4",
	} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		payload := []byte{0, 1, 2, 255}
		resp := &http.Response{StatusCode: http.StatusPartialContent, ContentLength: int64(len(payload)), Header: http.Header{
			"Content-Type": {contentType}, "Content-Range": {"bytes 0-3/4"},
			"Content-Disposition": {`attachment; filename="clip.mp4"`},
		}, Body: io.NopCloser(bytes.NewReader(payload))}
		require.NoError(t, writeGrokMediaContentResponse(c, resp))
		require.Equal(t, payload, rec.Body.Bytes())
		require.Equal(t, http.StatusPartialContent, rec.Code)
		require.Equal(t, "bytes 0-3/4", rec.Header().Get("Content-Range"))
		require.Equal(t, `attachment; filename="clip.mp4"`, rec.Header().Get("Content-Disposition"))
		want := "video/mp4"
		if contentType != want {
			want = "application/octet-stream"
		}
		require.Equal(t, want, rec.Header().Get("Content-Type"))
	}
}
