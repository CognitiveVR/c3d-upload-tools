package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

const testBoundary = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"

// writerOutput is what mime/multipart.Writer produces for parts with the
// given contents and boundary.
func writerOutput(t *testing.T, parts []Part, contents [][]byte, boundary string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.SetBoundary(boundary); err != nil {
		t.Fatal(err)
	}
	for i, p := range parts {
		contentType := p.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", multipart.FileContentDisposition(p.Field, p.Filename))
		header.Set("Content-Type", contentType)
		w, err := mw.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(contents[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// randomParts returns count parts of 0 to 64 KiB each, even indexes backed by
// files in a temporary directory and odd indexes held in memory. With more
// than two parts, the two middle parts (one of each kind) are empty.
func randomParts(t *testing.T, seed uint64, count int) ([]Part, [][]byte) {
	t.Helper()
	var key [32]byte
	key[0] = byte(seed)
	source := rand.NewChaCha8(key)
	rng := rand.New(source)
	dir := t.TempDir()
	contentTypes := []string{"", "application/octet-stream", "image/png", "application/json"}
	parts := make([]Part, count)
	contents := make([][]byte, count)
	for i := range count {
		content := make([]byte, rng.IntN(64<<10+1))
		if count > 2 && (i == count/2-1 || i == count/2) {
			content = nil
		}
		_, _ = source.Read(content)
		contents[i] = content
		part := Part{Field: "file", ContentType: contentTypes[i%len(contentTypes)], Size: int64(len(content))}
		if i%2 == 0 {
			part.Filename = fmt.Sprintf("part-%d.bin", i)
			path := filepath.Join(dir, part.Filename)
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			part.Open = func() (io.ReadCloser, error) { return os.Open(path) }
		} else {
			part.Field = "screenshot"
			part.Filename = fmt.Sprintf(`memory "%d"\part.png`, i)
			part.Open = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil }
		}
		parts[i] = part
	}
	return parts, contents
}

func TestMultipartBodyMatchesMultipartWriter(t *testing.T) {
	for _, tt := range []struct {
		count int
		seed  uint64
	}{{0, 1}, {1, 2}, {2, 3}, {50, 4}} {
		t.Run(fmt.Sprintf("%d parts seed %d", tt.count, tt.seed), func(t *testing.T) {
			parts, contents := randomParts(t, tt.seed, tt.count)
			want, wantType := writerOutput(t, parts, contents, testBoundary)

			body, contentType, contentLength, err := NewMultipartBody(parts, testBoundary)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("body differs from multipart.Writer output: got %d bytes, want %d", len(got), len(want))
			}
			if contentLength != int64(len(got)) {
				t.Errorf("contentLength %d, body is %d bytes", contentLength, len(got))
			}
			if contentType != wantType || contentType != "multipart/form-data; boundary="+testBoundary {
				t.Errorf("contentType %q, want %q", contentType, wantType)
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}

			again, _, _, err := NewMultipartBody(parts, testBoundary)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = again.Close() }()
			if err := iotest.TestReader(again, want); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNewMultipartBodyRejectsBadInput(t *testing.T) {
	ok := memoryPart("file", "cube.glb", "", "abc")
	if _, _, _, err := NewMultipartBody([]Part{ok}, "bad boundary!"); err == nil {
		t.Error("an invalid boundary was accepted")
	}
	if _, _, _, err := NewMultipartBody([]Part{ok}, ""); err == nil {
		t.Error("an empty boundary was accepted")
	}
	negative := ok
	negative.Size = -1
	_, _, _, err := NewMultipartBody([]Part{ok, negative}, testBoundary)
	if err == nil || !strings.Contains(err.Error(), "cube.glb") {
		t.Errorf("negative size: %v", err)
	}
}

func TestMultipartBodyFileSizeChanged(t *testing.T) {
	tests := []struct {
		name   string
		change func(path string) error
		want   string
	}{
		{"truncated", func(path string) error { return os.Truncate(path, 400) }, "read cube.glb: file size changed from 1000 to 400 bytes"},
		{"extended", func(path string) error { return os.WriteFile(path, make([]byte, 1500), 0o600) }, "read cube.glb: file size changed from 1000 to more than 1000 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cube.glb")
			if err := os.WriteFile(path, make([]byte, 1000), 0o600); err != nil {
				t.Fatal(err)
			}
			part := Part{Field: "file", Filename: "cube.glb", Size: 1000, Open: func() (io.ReadCloser, error) { return os.Open(path) }}
			body, _, _, err := NewMultipartBody([]Part{part}, testBoundary)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = body.Close() }()
			if err := tt.change(path); err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, body)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
			if _, again := body.Read(make([]byte, 10)); again == nil || again.Error() != tt.want {
				t.Errorf("later Read returned %v", again)
			}
			if body.BodySent() {
				t.Error("BodySent after a failed part")
			}
		})
	}
}

func TestMultipartBodyOpenAndReadErrorsNameTheFile(t *testing.T) {
	errMissing := errors.New("missing file")
	opening := Part{Field: "file", Filename: "cube.glb", Size: 3, Open: func() (io.ReadCloser, error) { return nil, errMissing }}
	body, _, _, err := NewMultipartBody([]Part{opening}, testBoundary)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, body)
	if !errors.Is(err, errMissing) || err.Error() != "open cube.glb: missing file" {
		t.Errorf("open failure: %v", err)
	}

	errDisk := errors.New("disk read failed")
	reading := Part{Field: "file", Filename: "cube.glb", Size: 100, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(errDisk))), nil
	}}
	body, _, _, err = NewMultipartBody([]Part{reading}, testBoundary)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, body)
	if !errors.Is(err, errDisk) || err.Error() != "read cube.glb: disk read failed" {
		t.Errorf("read failure: %v", err)
	}
}

// trackedOpener counts how often its part is opened and closed.
type trackedOpener struct {
	opens, closes int
}

type closeCounter struct {
	io.Reader
	closes *int
}

func (c *closeCounter) Close() error {
	*c.closes++
	return nil
}

func (o *trackedOpener) part(filename, content string) Part {
	return Part{Field: "file", Filename: filename, Size: int64(len(content)), Open: func() (io.ReadCloser, error) {
		o.opens++
		return &closeCounter{Reader: strings.NewReader(content), closes: &o.closes}, nil
	}}
}

func TestMultipartBodyOpensPartsLazilyAndClosesThem(t *testing.T) {
	var first, second trackedOpener
	parts := []Part{first.part("a.glb", "aaaa"), second.part("b.png", "bb")}
	want, _ := writerOutput(t, parts, [][]byte{[]byte("aaaa"), []byte("bb")}, testBoundary)
	firstHeader := bytes.Index(want, []byte("aaaa"))

	body, _, _, err := NewMultipartBody(parts, testBoundary)
	if err != nil {
		t.Fatal(err)
	}
	if first.opens != 0 || second.opens != 0 {
		t.Fatalf("opened before reading: %d %d", first.opens, second.opens)
	}
	if _, err := io.ReadFull(body, make([]byte, firstHeader)); err != nil {
		t.Fatal(err)
	}
	if first.opens != 0 {
		t.Fatalf("first part opened before its bytes were needed")
	}
	if _, err := io.ReadFull(body, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if first.opens != 1 || second.opens != 0 {
		t.Fatalf("after the first content byte: opens %d %d", first.opens, second.opens)
	}
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
	if first.opens != 1 || first.closes != 1 || second.opens != 1 || second.closes != 1 {
		t.Fatalf("opens %d %d closes %d %d", first.opens, second.opens, first.closes, second.closes)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if first.closes != 1 || second.closes != 1 {
		t.Fatalf("Close closed a finished part again: %d %d", first.closes, second.closes)
	}
}

func TestMultipartBodyClose(t *testing.T) {
	var opener trackedOpener
	parts := []Part{opener.part("a.glb", "aaaaaaaa")}
	want, _ := writerOutput(t, parts, [][]byte{[]byte("aaaaaaaa")}, testBoundary)
	body, _, _, err := NewMultipartBody(parts, testBoundary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(body, make([]byte, bytes.Index(want, []byte("aaaaaaaa"))+1)); err != nil {
		t.Fatal(err)
	}
	if opener.opens != 1 || opener.closes != 0 {
		t.Fatalf("opens %d closes %d", opener.opens, opener.closes)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if opener.closes != 1 {
		t.Fatalf("open part closed %d times", opener.closes)
	}
	if n, err := body.Read(make([]byte, 10)); n != 0 || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read after Close: %d %v", n, err)
	}
	if body.BodySent() {
		t.Fatal("BodySent after Close mid-body")
	}
}

func TestMultipartBodySent(t *testing.T) {
	parts := []Part{memoryPart("file", "cube.glb", "", "glb-bytes"), memoryPart("file", "settings.json", "application/json", "{}")}

	t.Run("whole body", func(t *testing.T) {
		body, _, _, err := NewMultipartBody(parts, testBoundary)
		if err != nil {
			t.Fatal(err)
		}
		if body.BodySent() {
			t.Fatal("BodySent before reading")
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			t.Fatal(err)
		}
		if !body.BodySent() {
			t.Fatal("BodySent false after the whole body was read")
		}
		_ = body.Close()
		if !body.BodySent() {
			t.Fatal("Close reset BodySent")
		}
	})

	t.Run("all but the last byte", func(t *testing.T) {
		body, _, contentLength, err := NewMultipartBody(parts, testBoundary)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = body.Close() }()
		if _, err := io.CopyN(io.Discard, body, contentLength-1); err != nil {
			t.Fatal(err)
		}
		if body.BodySent() {
			t.Fatal("BodySent before the final byte")
		}
		if _, err := io.ReadFull(body, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
		if !body.BodySent() {
			t.Fatal("BodySent false after the final byte")
		}
	})
}
