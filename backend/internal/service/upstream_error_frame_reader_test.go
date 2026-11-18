package service

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Generate arbitrarily many short data lines without allocating a giant input.
// There is deliberately no event boundary or EOF: the reader's error-event
// bound, rather than input exhaustion, must stop consumption.
type repeatingErrorDataReader struct {
	line   string
	offset int
	read   int
}

func (r *repeatingErrorDataReader) Read(target []byte) (int, error) {
	for n := 0; n < len(target); {
		copied := copy(target[n:], r.line[r.offset:])
		n += copied
		r.offset = (r.offset + copied) % len(r.line)
	}
	r.read += len(target)
	return len(target), nil
}

func TestUpstreamErrorFrameReaderOptionalErrorLimitBoundsConsumption(t *testing.T) {
	const limit = 64 << 10
	for _, header := range []string{"event: error\n", "id: test\nevent:error\n", "event: error\ndata: {\"type\":\"error\"}\n"} {
		t.Run(strings.TrimSpace(header), func(t *testing.T) {
			flood := &repeatingErrorDataReader{line: "data: \"incomplete multiline diagnostic\",\n"}
			source := io.MultiReader(strings.NewReader(header), flood)
			reader := newUpstreamErrorFrameReaderWithErrorLimit(source, 32<<20, limit)
			body, err := io.ReadAll(reader)
			require.ErrorIs(t, err, errUpstreamErrorFrameTooLarge)
			require.Empty(t, body, "oversized error content must not reach the downstream")
			require.LessOrEqual(t, flood.read, limit+8192, "the optional bound must apply before allocating the ordinary frame limit")
		})
	}
}

func TestUpstreamErrorFrameReaderErrorLimitDoesNotLimitNormalContent(t *testing.T) {
	const limit = 64 << 10
	content := `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"event: error ` + strings.Repeat("x", limit*2) + `"}}` + "\n\n"
	body, err := io.ReadAll(newUpstreamErrorFrameReaderWithErrorLimit(strings.NewReader(content), 1<<20, limit))
	require.NoError(t, err)
	require.Equal(t, content, string(body), "a string inside model content is not an SSE event header")
	// Every existing caller keeps the original behavior unless it explicitly
	// requests the error-specific cap.
	errorEvent := "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"" + strings.Repeat("x", limit*2) + "\"}}\n\n"
	body, err = io.ReadAll(newUpstreamErrorFrameReader(strings.NewReader(errorEvent), 1<<20))
	require.NoError(t, err)
	require.Equal(t, errorEvent, string(body))
}

func TestUpstreamErrorFrameLineReaderCombinesOnlyErrorData(t *testing.T) {
	const raw = `{"type":"error","error":{"type":"new_api_error","message":"bad input (request id: Synthetic123)"},"usage":{"input_tokens":13,"output_tokens":3}}`
	for _, tc := range []struct{ name, frame string }{
		{"named", "event: error\ndata: {\"type\":\"error\",\ndata: \"error\":{\"type\":\"new_api_error\",\"message\":\"bad input (request id: Synthetic123)\"},\ndata: \"usage\":{\"input_tokens\":13,\"output_tokens\":3}}\n\n"},
		{"data only", "data: {\"type\":\"error\",\ndata: \"error\":{\"type\":\"new_api_error\",\"message\":\"bad input (request id: Synthetic123)\"},\ndata: \"usage\":{\"input_tokens\":13,\"output_tokens\":3}}\n\n"},
		{"metadata and CRLF", "id: evt_safe\r\nevent:error\r\nretry: 1000\r\n: diagnostic\r\ndata: {\"type\":\"error\",\r\ndata: \"error\":{\"type\":\"new_api_error\",\"message\":\"bad input (request id: Synthetic123)\"},\r\ndata: \"usage\":{\"input_tokens\":13,\"output_tokens\":3}}\r\n\r\n"},
		{"EOF", "event: error\ndata: {\"type\":\"error\",\ndata: \"error\":{\"type\":\"new_api_error\",\"message\":\"bad input (request id: Synthetic123)\"},\ndata: \"usage\":{\"input_tokens\":13,\"output_tokens\":3}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := io.ReadAll(newUpstreamErrorFrameReaderForLineProcessing(strings.NewReader(tc.frame), 1<<20))
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(string(body), "data:"))
			require.Contains(t, string(body), "new_api_error", "internal parsing must retain the raw relay type")
			require.Contains(t, string(body), "request id: Synthetic123", "internal parsing must retain the raw correlation ID")
			var parser openAICompatSSEFrameParser
			var data string
			for _, line := range strings.Split(string(body), "\n") {
				if frame, ok := parser.AddLine(strings.TrimSuffix(line, "\r")); ok {
					data = frame.Data
				}
			}
			if frame, ok := parser.Finish(); ok {
				data = frame.Data
			}
			require.JSONEq(t, raw, data, "usage and diagnostic values must remain intact")
			if tc.name == "metadata and CRLF" {
				require.True(t, strings.HasPrefix(string(body), "id: evt_safe\r\nevent:error\r\nretry: 1000\r\n: diagnostic\r\n"))
				require.True(t, strings.HasSuffix(string(body), "\r\n\r\n"))
			}
			unchanged, err := io.ReadAll(newUpstreamErrorFrameReader(strings.NewReader(tc.frame), 1<<20))
			require.NoError(t, err)
			require.Equal(t, tc.frame, string(unchanged), "raw-frame consumers must keep their existing evidence bytes")
		})
	}
	for _, success := range []string{
		"event: content_block_delta\ndata: {\ndata: \"type\":\"content_block_delta\",\ndata: \"delta\":{\"text\":\"new_api_error (request id: Synthetic123)\"}}\n\n",
		"data: {\"candidates\":[\ndata: {\"content\":{\"parts\":[{\"text\":\"new_api_error (request id: Synthetic123)\"}]}}]}\r\n\r\n",
	} {
		body, err := io.ReadAll(newUpstreamErrorFrameReaderForLineProcessing(strings.NewReader(success), 1<<20))
		require.NoError(t, err)
		require.Equal(t, success, string(body), "successful multiline content must remain byte-for-byte unchanged")
	}
}
