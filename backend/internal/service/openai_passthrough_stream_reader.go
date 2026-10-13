package service

import (
	"bufio"
	"io"
	"sync/atomic"
	"time"
)

type openAIPassthroughScanEvent struct {
	line      string
	err       error
	processed chan struct{}
}

// Activity is measured on bytes read, not complete SSE lines: a large or slowly
// arriving JSON document must not look idle while its scanner is assembling it.
type openAIPassthroughActivityReader struct {
	io.ReadCloser
	lastRead atomic.Int64
}

func newOpenAIPassthroughActivityReader(body io.ReadCloser) *openAIPassthroughActivityReader {
	reader := &openAIPassthroughActivityReader{ReadCloser: body}
	reader.lastRead.Store(time.Now().UnixNano())
	return reader
}

// A transforming body can buffer raw bytes without emitting a complete line.
// Carry its raw source's activity into the downstream scanner's idle loop.
type openAIPassthroughActivityBody struct {
	io.ReadCloser
	activity *openAIPassthroughActivityReader
}

func (r *openAIPassthroughActivityReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

func startOpenAIPassthroughScanner(body io.ReadCloser, maxLineSize int) (*openAIPassthroughActivityReader, <-chan openAIPassthroughScanEvent, func()) {
	var reader *openAIPassthroughActivityReader
	var scanReader io.Reader
	if tracked, ok := body.(*openAIPassthroughActivityBody); ok {
		reader = tracked.activity
		scanReader = body
	} else {
		reader = newOpenAIPassthroughActivityReader(body)
		scanReader = reader
	}
	events := make(chan openAIPassthroughScanEvent)
	done := make(chan struct{})
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		defer close(events)
		scanBuf := getSSEScannerBuf64K()
		defer putSSEScannerBuf64K(scanBuf)
		scanner := bufio.NewScanner(scanReader)
		attachSSEScannerBuffer(scanner, scanBuf[:], maxLineSize)
		documents := newOpenAISSEJSONDocumentScanner(scanner)
		send := func(event openAIPassthroughScanEvent) bool {
			event.processed = make(chan struct{})
			select {
			case events <- event:
			case <-done:
				return false
			}
			select {
			case <-event.processed:
				return true
			case <-done:
				return false
			}
		}
		for documents.Scan() {
			if !send(openAIPassthroughScanEvent{line: documents.Text()}) {
				return
			}
		}
		if err := documents.Err(); err != nil {
			send(openAIPassthroughScanEvent{err: err})
		}
	}()
	return reader, events, func() {
		close(done)
		_ = body.Close()
		// A misbehaving custom body may ignore Close. It retains its own buffer
		// until Read returns; never recycle memory still owned by the scanner.
		timer := time.NewTimer(openAIStreamScannerShutdownTimeout)
		defer timer.Stop()
		select {
		case <-scanDone:
		case <-timer.C:
		}
	}
}
