package handler

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"go.uber.org/zap"
)

// logRequestBodyReadFailure records a bounded, payload-free reason for a body
// read failure. Clients continue to receive the stable generic error message;
// operators get enough information to distinguish compression failures from a
// disconnected/truncated upload without logging request content.
func logRequestBodyReadFailure(reqLog *zap.Logger, req *http.Request, err error) {
	if reqLog == nil || err == nil {
		return
	}

	contentLength := int64(-1)
	contentEncoding := "identity"
	if req != nil {
		contentLength = req.ContentLength
		contentEncoding = requestContentEncodingCategory(req.Header.Get("Content-Encoding"))
	}

	reqLog.Warn("read request body failed",
		zap.String("error_kind", requestBodyReadErrorKind(err)),
		zap.String("content_encoding", contentEncoding),
		zap.Int64("content_length", contentLength),
	)
}

func requestContentEncodingCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "identity":
		return "identity"
	case "gzip", "x-gzip":
		return "gzip"
	case "zstd":
		return "zstd"
	case "deflate":
		return "deflate"
	default:
		return "other"
	}
}

func requestBodyReadErrorKind(err error) string {
	if err == nil {
		return "none"
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return "max_bytes"
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "decode content-encoding") {
		if strings.Contains(lower, "unsupported content-encoding") {
			return "unsupported_content_encoding"
		}
		return "decode_content_encoding"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return "client_disconnect"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "truncated_body"
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	if errors.As(err, &netErr) {
		return "transport"
	}
	return "io_read"
}

// requestBodyReadFailure shares the payload-free classifier with read logging.
func requestBodyReadFailure(reqLog *zap.Logger, req *http.Request, err error) (int, string) {
	logRequestBodyReadFailure(reqLog, req, err)
	if maxErr, ok := extractMaxBytesError(err); ok {
		return http.StatusRequestEntityTooLarge, buildBodyTooLargeMessage(maxErr.Limit)
	}
	if req != nil && errors.Is(req.Context().Err(), context.Canceled) {
		return 499, "Client closed request"
	}
	switch requestBodyReadErrorKind(err) {
	case "timeout":
		return http.StatusRequestTimeout, "Timed out reading request body"
	case "client_disconnect":
		return 499, "Client closed request"
	default:
		return http.StatusBadRequest, "Failed to read request body"
	}
}
