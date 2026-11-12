package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"sync"
	"sync/atomic"
)

// MultipartBody streams a multipart/form-data body whose length is fixed
// before the first byte is read: each part must yield exactly Part.Size
// bytes. Read may be called concurrently with Close.
type MultipartBody struct {
	parts   []Part
	headers [][]byte
	closing []byte

	mu        sync.Mutex
	index     int // part being sent; len(parts) once the closing boundary is due
	inContent bool
	offset    int // bytes of the current header or closing boundary returned
	current   io.ReadCloser
	read      int64 // bytes of the current part returned
	err       error
	closed    bool
	sent      atomic.Bool
}

// NewMultipartBody plans a body over parts with boundary. Parts are opened
// one at a time, when their bytes are first needed.
func NewMultipartBody(parts []Part, boundary string) (*MultipartBody, string, int64, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, "", 0, fmt.Errorf("multipart boundary: %w", err)
	}
	b := &MultipartBody{parts: append([]Part(nil), parts...), headers: make([][]byte, len(parts))}
	var length int64
	for i, part := range parts {
		if part.Size < 0 {
			return nil, "", 0, fmt.Errorf("part %s has negative size %d", part.Filename, part.Size)
		}
		if _, err := mw.CreatePart(partHeader(part)); err != nil {
			return nil, "", 0, err
		}
		b.headers[i] = bytes.Clone(buf.Bytes())
		buf.Reset()
		length += int64(len(b.headers[i])) + part.Size
	}
	if err := mw.Close(); err != nil {
		return nil, "", 0, err
	}
	b.closing = bytes.Clone(buf.Bytes())
	length += int64(len(b.closing))
	return b, mw.FormDataContentType(), length, nil
}

func partHeader(part Part) textproto.MIMEHeader {
	contentType := part.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", multipart.FileContentDisposition(part.Field, part.Filename))
	header.Set("Content-Type", contentType)
	return header
}

// BodySent reports whether the final byte of the closing boundary has been
// returned by Read.
func (b *MultipartBody) BodySent() bool { return b.sent.Load() }

func (b *MultipartBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		if b.closed {
			return 0, io.ErrClosedPipe
		}
		if b.err != nil {
			return 0, b.err
		}
		if b.index == len(b.parts) {
			if b.offset == len(b.closing) {
				return 0, io.EOF
			}
			n := copy(p, b.closing[b.offset:])
			b.offset += n
			if b.offset == len(b.closing) {
				b.sent.Store(true)
			}
			return n, nil
		}
		if !b.inContent {
			header := b.headers[b.index]
			n := copy(p, header[b.offset:])
			b.offset += n
			if b.offset == len(header) {
				b.inContent, b.offset = true, 0
			}
			return n, nil
		}
		if n, err := b.readContent(p); n > 0 || err != nil {
			return n, err
		}
	}
}

// readContent is called with b.mu held and releases it while the part is
// read, so Close can interrupt a slow source.
func (b *MultipartBody) readContent(p []byte) (int, error) {
	part := b.parts[b.index]
	if b.current == nil {
		rc, err := part.Open()
		if err != nil {
			return 0, b.fail(fmt.Errorf("open %s: %w", part.Filename, err))
		}
		b.current = rc
	}
	remaining := part.Size - b.read
	buf := p
	if remaining == 0 {
		buf = p[:1]
	} else if int64(len(buf)) > remaining {
		buf = buf[:remaining]
	}
	source := b.current
	b.mu.Unlock()
	n, err := source.Read(buf)
	b.mu.Lock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	if remaining == 0 && n > 0 {
		return 0, b.fail(fmt.Errorf("read %s: file size changed from %d to more than %d bytes", part.Filename, part.Size, part.Size))
	}
	b.read += int64(n)
	switch {
	case err == nil:
		return n, nil
	case !errors.Is(err, io.EOF):
		return n, b.fail(fmt.Errorf("read %s: %w", part.Filename, err))
	case b.read < part.Size:
		return n, b.fail(fmt.Errorf("read %s: file size changed from %d to %d bytes", part.Filename, part.Size, b.read))
	}
	b.closeCurrent()
	b.index++
	b.inContent, b.read = false, 0
	return n, nil
}

func (b *MultipartBody) fail(err error) error {
	b.err = err
	b.closeCurrent()
	return err
}

func (b *MultipartBody) closeCurrent() {
	if b.current != nil {
		_ = b.current.Close()
		b.current = nil
	}
}

// failure is the local error that stopped the body, if any.
func (b *MultipartBody) failure() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

// Close closes the open part, if any; later reads fail with
// io.ErrClosedPipe.
func (b *MultipartBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		b.closeCurrent()
	}
	return nil
}
