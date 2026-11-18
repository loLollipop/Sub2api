package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type upstreamErrorFrameReader struct {
	reader           *bufio.Reader
	limit            int64
	pending          []byte
	err              error
	errorLimit       int64
	compactErrorData bool
}

var errUpstreamErrorFrameTooLarge = errors.New("upstream error event exceeds size limit")

func newUpstreamErrorFrameReader(reader io.Reader, limit int64) io.Reader {
	return &upstreamErrorFrameReader{reader: bufio.NewReader(reader), limit: limit}
}

// Line-oriented consumers need a complete error JSON document before recording
// diagnostics, classifying the failure and sanitizing their client copy. Keep
// successful frames unchanged and leave raw-frame consumers on the normal reader.
func newUpstreamErrorFrameReaderForLineProcessing(reader io.Reader, limit int64) io.Reader {
	return &upstreamErrorFrameReader{reader: bufio.NewReader(reader), limit: limit, compactErrorData: true}
}

// A separate optional limit bounds named SSE errors before the frame buffer
// allocates the much larger limit used for regular model output.
func newUpstreamErrorFrameReaderWithErrorLimit(reader io.Reader, limit, errorLimit int64) io.Reader {
	return &upstreamErrorFrameReader{reader: bufio.NewReader(reader), limit: limit, errorLimit: errorLimit}
}

func (reader *upstreamErrorFrameReader) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	for len(reader.pending) == 0 {
		if reader.err != nil {
			return 0, reader.err
		}
		frame, err := reader.readFrame()
		reader.err = err
		reader.pending = frame
	}
	count := copy(target, reader.pending)
	reader.pending = reader.pending[count:]
	return count, nil
}

func (reader *upstreamErrorFrameReader) readFrame() ([]byte, error) {
	var frame bytes.Buffer
	var parser openAICompatSSEFrameParser
	dataLines := 0
	plainJSON := false
	lineStart := 0
	namedError := false
	for {
		line, err := reader.reader.ReadSlice('\n')
		if namedError && reader.errorLimit > 0 && int64(frame.Len()+len(line)) > reader.errorLimit {
			return nil, errUpstreamErrorFrameTooLarge
		}
		if int64(frame.Len()+len(line)) > reader.limit {
			return nil, bufio.ErrTooLong
		}
		if frame.Len() == 0 && err != bufio.ErrBufferFull {
			text := strings.TrimRight(string(line), "\r\n")
			if text == "" || strings.HasPrefix(text, ":") {
				return line, err
			}
			if data, ok := extractOpenAISSEDataLine(text); ok && (data == "[DONE]" || json.Valid([]byte(data))) {
				return line, err
			}
		}
		_, _ = frame.Write(line)
		if err == bufio.ErrBufferFull {
			continue
		}
		line = frame.Bytes()[lineStart:]
		lineStart = frame.Len()
		if dataLines == 0 {
			trimmed := bytes.TrimSpace(frame.Bytes())
			plainJSON = bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))
		}
		if plainJSON {
			if err != nil || json.Valid(frame.Bytes()) {
				body := frame.Bytes()
				if upstreamFinancialFailureEnvelope(body) {
					var failure any = json.RawMessage(body)
					if !json.Valid(body) {
						failure = string(body)
					}
					payload, _ := json.Marshal(map[string]any{"type": "error", "error": failure})
					return append(append([]byte("data: "), payload...), '\n', '\n'), err
				}
				return body, err
			}
			continue
		}
		text := strings.TrimRight(string(line), "\r\n")
		if eventName, ok := strings.CutPrefix(text, "event:"); ok && strings.TrimSpace(eventName) == "error" {
			namedError = true
			if reader.errorLimit > 0 && int64(frame.Len()) > reader.errorLimit {
				return nil, errUpstreamErrorFrameTooLarge
			}
		}
		if frame.Len() == len(line) && strings.HasPrefix(text, ":") {
			return frame.Bytes(), err
		}
		if data, ok := extractOpenAISSEDataLine(text); ok {
			dataLines++
			// Keep a bounded named error together through its event boundary.
			// Otherwise a valid first data line would reset namedError before a
			// later oversized line, bypassing the smaller diagnostic limit.
			if dataLines == 1 && (!namedError || reader.errorLimit == 0 && !reader.compactErrorData) && (data == "[DONE]" || json.Valid([]byte(data))) {
				return frame.Bytes(), err
			}
		}
		parsed, ready := parser.AddLine(text)
		if err != nil && !ready {
			parsed, ready = parser.Finish()
		}
		if text == "" || err != nil {
			if ready && dataLines > 1 && (upstreamFinancialFailureEnvelope([]byte(parsed.Data)) || parsed.EventType == "error" && IsUpstreamFinancialError(0, []byte(parsed.Data)) ||
				reader.compactErrorData && (parsed.EventType == "error" || clientUpstreamBodyIsError([]byte(parsed.Data)))) {
				var compact bytes.Buffer
				if json.Compact(&compact, []byte(parsed.Data)) == nil {
					if reader.compactErrorData {
						return compactUpstreamSSEErrorData(frame.Bytes(), compact.Bytes()), err
					}
					return []byte("event: error\ndata: " + compact.String() + "\n\n"), err
				}
			}
			return frame.Bytes(), err
		}
	}
}

// Replace only the data lines. Event names, IDs, retry hints, comments and line
// endings retain their original framing; the payload itself is still raw JSON.
func compactUpstreamSSEErrorData(frame, payload []byte) []byte {
	var result bytes.Buffer
	wroteData := false
	for _, line := range bytes.SplitAfter(frame, []byte("\n")) {
		text := strings.TrimRight(string(line), "\r\n")
		if _, ok := extractOpenAISSEDataLine(text); ok {
			if !wroteData {
				_, _ = result.WriteString("data: ")
				_, _ = result.Write(payload)
				_, _ = result.Write(line[len(text):])
				wroteData = true
			}
			continue
		}
		_, _ = result.Write(line)
	}
	return result.Bytes()
}
