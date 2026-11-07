package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImagesHTTP200ErrorPrivacyAndNoBillingResult(t *testing.T) {
	for _, responseBody := range []string{
		`image generation failed: POST https://private-upstream.example/v1/images?key=synthetic-credential failed`,
		`{"error":{"type":"invalid_request_error","code":"invalid_value","param":"size","message":"Invalid size; https://private-upstream.example/v1/images?key=synthetic-credential"},"usage":{"input_tokens":999,"output_tokens":1}}`,
	} {
		body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"}`)
		c, rec := newOpenAIImagesTestContext(t, body)
		recorder := &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(responseBody)),
		}}
		svc := newOpenAIImagesTestService(recorder)
		parsed, err := svc.ParseOpenAIImagesRequest(c, body)
		require.NoError(t, err)
		result, err := svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyAccount(), body, parsed, "")
		require.Nil(t, result, "a failed image must not enter successful usage settlement")
		var upstreamErr *OpenAIImagesUpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		require.Contains(t, upstreamErr.Message, "private-upstream.example", "internal classification retains original diagnostics")
		require.NotContains(t, rec.Body.String(), "private-upstream")
		require.NotContains(t, rec.Body.String(), "synthetic-credential")
		require.Contains(t, rec.Body.String(), upstreamErr.Code)
		require.Equal(t, upstreamErr.StatusCode, rec.Code)
	}
}

func TestOpenAIImagesHTTP200ErrorPreservesSuccessfulContent(t *testing.T) {
	body := `{"data":[{"b64_json":"aGVsbG8=","revised_prompt":"image generation failed at https://public.example.test"}],"message":"invalid size"}`
	require.Nil(t, openAIImagesHTTP200Error([]byte(body), nil))
	require.Nil(t, openAIImagesHTTP200Error([]byte(`{"data":[{"url":"https://public.example.test/a.png"}],"error":{"message":"invalid size"}}`), nil))
	c, rec := newOpenAIImagesTestContext(t, nil)
	svc := &OpenAIGatewayService{}
	_, count, _, err := svc.handleOpenAIImagesNonStreamingResponse(context.Background(), &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)),
	}, c, newOpenAIImagesAPIKeyAccount(), &OpenAIImagesRequest{})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.JSONEq(t, body, rec.Body.String(), "successful prompt text must pass through unchanged")
}

func TestOpenAIImagesBoundedErrorMessagePreservesUTF8(t *testing.T) {
	message := openAIImagesBoundedErrorMessage(strings.Repeat("图", 1000))
	require.True(t, utf8.ValidString(message))
	require.LessOrEqual(t, len(message), 2000+len("...(truncated)"))
}

func TestOpenAIImagesHTTP200ErrorSizeFailureDoesNotCoolAccount(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"Invalid size '4096x4096'."}}`,
		`{"type":"error","error":{"message":"Invalid size '4096x4096'."}}`,
		`{"type":"response.failed","response":{"error":{"message":"Invalid size '4096x4096'."}}}`,
	} {
		err := openAIImagesHTTP200Error([]byte(body), nil)
		require.NotNil(t, err)
		require.Equal(t, http.StatusBadRequest, err.StatusCode)
		require.Equal(t, "invalid_request_error", err.ErrorType)
		require.False(t, IsOpenAIImagesRetryableUpstreamError(err))
		_, _, unhealthy := classifyOpenAIAPIKeyHealthFailure(err)
		require.False(t, unhealthy, "a request size rejection must not cool a healthy account")
	}
}

const gifSheetUpstreamTextError = `No supported sheet fits 16 GIF frames at size '1024x1024'.
Invalid size '4096x4096'.
The longest edge must be less than or equal to 3840.`

func gifSheetTestAccount(baseURL string) *Account {
	return &Account{
		ID:       77,
		Name:     "gif-sheet-relay",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": baseURL,
		},
	}
}

func gifSheetTestParsed(frameCount int, size, outputFormat string) *OpenAIImagesRequest {
	count := frameCount
	return &OpenAIImagesRequest{
		Endpoint:     openAIImagesGenerationsEndpoint,
		Model:        "gpt-image-2.6-sunburst",
		Size:         size,
		SizeTier:     normalizeOpenAIImageSizeTier(size),
		OutputFormat: outputFormat,
		N:            1,
		FrameCount:   &count,
	}
}

func TestOpenAIImagesHTTP200Error(t *testing.T) {
	requestHeader := http.Header{"X-Request-Id": []string{"ERR-711DE84247"}}
	plainHeader := http.Header{"X-Request-Id": []string{"req_text_error"}}

	t.Run("plain text sheet error is recognized", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error([]byte(gifSheetUpstreamTextError), requestHeader)
		require.NotNil(t, upErr)
		require.Equal(t, http.StatusBadRequest, upErr.clientStatusCode())
		require.Equal(t, "invalid_request_error", upErr.clientErrorType())
		require.Equal(t, "ERR-711DE84247", upErr.Code)
		require.Equal(t, "ERR-711DE84247", upErr.UpstreamRequestID)
		require.Contains(t, upErr.Message, "No supported sheet fits 16 GIF frames")
		require.Contains(t, upErr.Message, "longest edge must be less than or equal to 3840")
	})

	t.Run("plain text error code without ERR header", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error([]byte("image generation failed: backend unavailable"), plainHeader)
		require.NotNil(t, upErr)
		require.Equal(t, "upstream_error_response", upErr.Code)
		require.Equal(t, "req_text_error", upErr.UpstreamRequestID)
		require.Contains(t, upErr.Message, "backend unavailable")
	})

	t.Run("normal b64_json payload is not an error", func(t *testing.T) {
		require.Nil(t, openAIImagesHTTP200Error(
			[]byte(`{"created":1,"data":[{"b64_json":"aGVsbG8="}]}`), requestHeader))
	})

	t.Run("normal url payload is not an error", func(t *testing.T) {
		require.Nil(t, openAIImagesHTTP200Error(
			[]byte(`{"created":1,"data":[{"url":"https://example.test/a.png"}]}`), requestHeader))
	})

	t.Run("json error envelope keeps upstream status", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error(
			[]byte(`{"error":{"type":"invalid_request_error","code":"invalid_value","message":"Invalid size '4096x4096'."}}`),
			plainHeader,
		)
		require.NotNil(t, upErr)
		require.Equal(t, http.StatusBadRequest, upErr.clientStatusCode())
		require.Equal(t, "invalid_value", upErr.Code)
		require.Contains(t, upErr.Message, "Invalid size '4096x4096'")
	})

	t.Run("json error message without type keeps request-shape status", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error(
			[]byte(`{"error":{"message":"No supported sheet fits 16 GIF frames at size '1024x1024'."}}`),
			plainHeader,
		)
		require.NotNil(t, upErr)
		require.Equal(t, http.StatusBadRequest, upErr.clientStatusCode())
		require.Contains(t, upErr.Message, "No supported sheet fits")
	})

	t.Run("json detail with recognized text is an error", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error(
			[]byte(`{"detail":"No supported sheet fits 16 GIF frames at size '1024x1024'."}`),
			plainHeader,
		)
		require.NotNil(t, upErr)
		require.Equal(t, http.StatusBadRequest, upErr.clientStatusCode())
		require.Contains(t, upErr.Message, "No supported sheet fits")
	})

	t.Run("json type error is an error", func(t *testing.T) {
		upErr := openAIImagesHTTP200Error(
			[]byte(`{"type":"error","error":{"type":"server_error","code":"server_error","message":"image backend unavailable"}}`),
			plainHeader,
		)
		require.NotNil(t, upErr)
		require.Equal(t, http.StatusInternalServerError, upErr.clientStatusCode())
		require.Equal(t, "req_text_error", upErr.UpstreamRequestID)
		require.Contains(t, upErr.Message, "image backend unavailable")
	})

	t.Run("empty data array keeps existing passthrough behavior", func(t *testing.T) {
		require.Nil(t, openAIImagesHTTP200Error([]byte(`{"created":1,"data":[]}`), plainHeader))
	})

	t.Run("unrelated json message is not an error", func(t *testing.T) {
		require.Nil(t, openAIImagesHTTP200Error([]byte(`{"message":"all good"}`), plainHeader))
	})

	t.Run("unrelated plain text is not an error", func(t *testing.T) {
		require.Nil(t, openAIImagesHTTP200Error([]byte("service warming up"), plainHeader))
	})
}

// 线上复现：中转站用 HTTP 200 + 纯文本错误返回失败，旧逻辑会 c.Data(200) 并把
// ImageCount 记成 1。修复后必须返回错误、不产出 ForwardResult（handler 因此不会
// 进入 RecordUsage），客户端拿到对应错误状态和脱敏后的 message。
func TestHandleOpenAIImagesNonStreamingResponse_TextErrorIsNotSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, rec := newOpenAIImagesTestContext(t, nil)
	parsed := gifSheetTestParsed(16, "864x864", "gif")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Request-Id": []string{"ERR-87ADE609BA"},
		},
		Body: io.NopCloser(strings.NewReader(gifSheetUpstreamTextError)),
	}

	svc := &OpenAIGatewayService{}
	usage, imageCount, sizes, err := svc.handleOpenAIImagesNonStreamingResponse(
		context.Background(), resp, c, gifSheetTestAccount("https://image-relay.example.test/v1"), parsed,
	)

	require.Error(t, err)
	var upErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upErr)
	require.Equal(t, http.StatusBadRequest, upErr.StatusCode)
	require.Equal(t, "invalid_request_error", upErr.ErrorType)
	require.Equal(t, "ERR-87ADE609BA", upErr.Code)
	require.Equal(t, "ERR-87ADE609BA", upErr.UpstreamRequestID)
	require.Contains(t, upErr.Message, "No supported sheet fits")
	require.Contains(t, upErr.Message, "longest edge must be less than or equal to 3840")
	require.Zero(t, imageCount, "失败响应不得按 1 张图计费")
	require.Empty(t, sizes)
	require.Zero(t, usage.InputTokens+usage.OutputTokens)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "No supported sheet fits")
}

// 200 + 正常 data[].b64_json 不能被错误识别误伤。
func TestHandleOpenAIImagesNonStreamingResponse_NormalImageStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, rec := newOpenAIImagesTestContext(t, nil)
	parsed := gifSheetTestParsed(8, "864x864", "gif")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","output_format":"gif","size":"864x864"}]}`,
		)),
	}

	svc := &OpenAIGatewayService{}
	_, imageCount, _, err := svc.handleOpenAIImagesNonStreamingResponse(
		context.Background(), resp, c, gifSheetTestAccount("https://image-relay.example.test/v1"), parsed,
	)

	require.NoError(t, err)
	require.Equal(t, 1, imageCount)
	require.Equal(t, http.StatusOK, rec.Code)
}

// 走完整的 API Key 转发路径：上游 200 + 文本错误时 ForwardImages 返回 nil,error，
// handler 的 record usage 分支不会执行。
func TestForwardOpenAIImagesAPIKey_TextErrorReturnsNilResult(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2.6-sunburst","prompt":"draw a lantern","size":"1024x1024","output_format":"png"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	recorder := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Request-Id": []string{"ERR-711DE84247"},
		},
		Body: io.NopCloser(strings.NewReader("image generation failed: backend unavailable")),
	}}
	svc := newOpenAIImagesTestService(recorder)

	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(
		context.Background(), c, gifSheetTestAccount("https://image-relay.example.test/v1"), body, parsed, "",
	)

	require.Nil(t, result, "失败响应不得返回 ForwardResult，否则 handler 会按成功计费")
	require.Error(t, err)
	var upErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upErr)
	require.Equal(t, http.StatusBadGateway, upErr.StatusCode)
	require.Equal(t, "ERR-711DE84247", upErr.Code)
	require.Len(t, recorder.requests, 1, "上游请求本身已经发出")
	require.Equal(t, http.StatusBadGateway, rec.Code)
}
