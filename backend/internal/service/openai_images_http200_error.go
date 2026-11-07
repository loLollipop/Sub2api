package service

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// openAIImagesTextErrorMarkers are the plain-text failure shapes image relays
// have been observed to return with HTTP 200. A non-JSON 200 body is only
// reclassified when it carries one of these markers; a successful Images
// response is JSON with data[].b64_json/url and never matches.
var openAIImagesTextErrorMarkers = []string{
	"no supported sheet fits",
	"invalid size",
	"longest edge must be less than or equal to",
	"gif frames",
	"image generation failed",
	"failed to generate image",
	"unable to generate image",
	"unsupported image size",
	"image size is not supported",
	"requested size is not supported",
}

var openAIImagesUpstreamERRCodePattern = regexp.MustCompile(`ERR-[0-9A-Z]{6,}`)

// openAIImagesLookLikeTextError reports whether a non-JSON Images response body
// is a recognizable error message instead of image data.
func openAIImagesLookLikeTextError(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range openAIImagesTextErrorMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return openAIImagesUpstreamERRCodePattern.MatchString(text)
}

// openAIImagesUpstreamRequestID returns the traceable upstream request id from
// the response headers, if the relay sent one.
func openAIImagesUpstreamRequestID(header http.Header) string {
	if header == nil {
		return ""
	}
	for _, key := range []string{"x-request-id", "request-id", "x-upstream-request-id"} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

// openAIImagesTextErrorStatus distinguishes a request-shape rejection (the
// upstream says the requested size/frame combination is invalid) from a
// generic upstream failure. The former must not cool the image account or be
// retried on another account.
func openAIImagesTextErrorStatus(text string) int {
	lower := strings.ToLower(text)
	for _, marker := range []string{
		"no supported sheet fits",
		"invalid size",
		"longest edge must be less than or equal to",
		"gif frames",
		"unsupported image size",
		"image size is not supported",
		"requested size is not supported",
	} {
		if strings.Contains(lower, marker) {
			return http.StatusBadRequest
		}
	}
	return http.StatusBadGateway
}

func openAIImagesTextErrorType(status int) string {
	if status == http.StatusBadRequest {
		return "invalid_request_error"
	}
	return "upstream_error"
}

// A relay may omit type/code and only send the message. Recognized size/frame
// errors are request-scoped (400), regardless of their envelope nesting.
func classifyOpenAIImagesHTTP200StructuredError(err *OpenAIImagesUpstreamError) *OpenAIImagesUpstreamError {
	if err.StatusCode == http.StatusBadGateway && openAIImagesLookLikeTextError(err.Message) {
		err.StatusCode = openAIImagesTextErrorStatus(err.Message)
		err.ErrorType = openAIImagesTextErrorType(err.StatusCode)
	}
	return err
}

// openAIImagesTextErrorCode keeps the relay's own ERR-xxxxxxxx code visible to
// clients and logs; the code may sit in the body or in the request-id header.
func openAIImagesTextErrorCode(text, requestID string) string {
	if code := openAIImagesUpstreamERRCodePattern.FindString(text); code != "" {
		return code
	}
	if code := openAIImagesUpstreamERRCodePattern.FindString(requestID); code != "" {
		return code
	}
	return "upstream_error_response"
}

func openAIImagesBoundedErrorMessage(text string) string {
	const maxMessageBytes = 2000
	text = strings.TrimSpace(text)
	if len(text) > maxMessageBytes {
		end := maxMessageBytes
		for !utf8.RuneStart(text[end]) {
			end--
		}
		return text[:end] + "...(truncated)"
	}
	return text
}

// openAIImagesResponseHasImageOutput reports whether a JSON Images response
// carries the data[].b64_json/url payload of a successful generation.
func openAIImagesResponseHasImageOutput(root gjson.Result) bool {
	items := root.Get("data")
	if !items.IsArray() {
		return false
	}
	for _, item := range items.Array() {
		if strings.TrimSpace(item.Get("b64_json").String()) != "" {
			return true
		}
		if strings.TrimSpace(item.Get("url").String()) != "" {
			return true
		}
	}
	return false
}

// openAIImagesHTTP200Error 识别「HTTP 200 但响应体其实是错误」的上游响应。
//
// 中转站可能把失败的生图请求用 200 + 纯文本（或 200 + {"error":...}）返回。
// 旧逻辑只看状态码，于是把这段错误文本当作成功响应转发、并按 1 张图计费。
// 这里只认图片接口可识别的失败形状：
//   - 已有 data[].b64_json/url 的 JSON：一律视为成功，绝不改写；
//   - JSON 里的 error / type=error / response.failed / message / detail；
//   - 带生图错误标记或 ERR- 代码的纯文本。
//
// 返回非 nil 时调用方必须按错误响应处理，不得进入计费。
func openAIImagesHTTP200Error(body []byte, header http.Header) *OpenAIImagesUpstreamError {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil
	}
	requestID := openAIImagesUpstreamRequestID(header)
	if gjson.ValidBytes(trimmed) {
		root := gjson.ParseBytes(trimmed)
		if openAIImagesResponseHasImageOutput(root) {
			return nil
		}
		if errorObj := root.Get("error"); errorObj.Exists() {
			if errorObj.IsObject() {
				if upstreamErr := openAIImagesUpstreamErrorFromGJSON(errorObj, requestID); upstreamErr != nil {
					return classifyOpenAIImagesHTTP200StructuredError(upstreamErr)
				}
			}
			if errorObj.Type == gjson.String {
				message := strings.TrimSpace(errorObj.String())
				if message != "" {
					status := openAIImagesTextErrorStatus(message)
					return &OpenAIImagesUpstreamError{
						StatusCode:        status,
						ErrorType:         openAIImagesTextErrorType(status),
						Code:              openAIImagesTextErrorCode(message, requestID),
						Message:           sanitizeUpstreamErrorMessage(openAIImagesBoundedErrorMessage(message)),
						UpstreamRequestID: requestID,
					}
				}
			}
		}
		switch root.Get("type").String() {
		case "error", "response.failed":
			if upstreamErr := openAIImagesUpstreamErrorFromSSEPayload(trimmed); upstreamErr != nil {
				if upstreamErr.UpstreamRequestID == "" {
					upstreamErr.UpstreamRequestID = requestID
				}
				return classifyOpenAIImagesHTTP200StructuredError(upstreamErr)
			}
		}
		for _, field := range []string{"message", "detail"} {
			value := root.Get(field)
			if value.Type != gjson.String {
				continue
			}
			message := strings.TrimSpace(value.String())
			if message == "" || !openAIImagesLookLikeTextError(message) {
				continue
			}
			status := openAIImagesTextErrorStatus(message)
			return &OpenAIImagesUpstreamError{
				StatusCode:        status,
				ErrorType:         openAIImagesTextErrorType(status),
				Code:              openAIImagesTextErrorCode(message, requestID),
				Message:           sanitizeUpstreamErrorMessage(openAIImagesBoundedErrorMessage(message)),
				UpstreamRequestID: requestID,
			}
		}
		return nil
	}

	text := strings.TrimSpace(string(trimmed))
	if !openAIImagesLookLikeTextError(text) {
		return nil
	}
	status := openAIImagesTextErrorStatus(text)
	return &OpenAIImagesUpstreamError{
		StatusCode:        status,
		ErrorType:         openAIImagesTextErrorType(status),
		Code:              openAIImagesTextErrorCode(text, requestID),
		Message:           sanitizeUpstreamErrorMessage(openAIImagesBoundedErrorMessage(text)),
		UpstreamRequestID: requestID,
	}
}
