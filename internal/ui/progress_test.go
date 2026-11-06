package ui

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestProgress(w io.Writer, total int64, label string) (*Progress, *fakeClock) {
	c := &fakeClock{t: time.Unix(1000, 0)}
	p := NewProgress(w, total, label)
	p.now = c.now
	return p, c
}

func TestProgressLineFormat(t *testing.T) {
	var buf bytes.Buffer
	p, _ := newTestProgress(&buf, 290_000_000, "scene.glb")
	p.Add(123_000_000)
	want := "\rscene.glb: 42% 123 MB of 290 MB"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

func TestProgressThrottle(t *testing.T) {
	var buf bytes.Buffer
	p, c := newTestProgress(&buf, 1000, "x")
	p.Add(100)
	first := buf.Len()
	c.t = c.t.Add(50 * time.Millisecond)
	p.Add(100)
	if buf.Len() != first {
		t.Fatalf("redrew within 100 milliseconds: %q", buf.String())
	}
	c.t = c.t.Add(50 * time.Millisecond)
	p.Add(100)
	if buf.Len() == first {
		t.Fatal("did not redraw after 100 milliseconds")
	}
	if !strings.HasSuffix(buf.String(), "30% 300 B of 1.0 KB") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestProgressDone(t *testing.T) {
	var buf bytes.Buffer
	p, _ := newTestProgress(&buf, 2_000_000, "x")
	p.Add(10)
	buf.Reset()
	p.Done()
	if buf.String() != "\rx: 100% 2.0 MB of 2.0 MB\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestProgressZeroTotalOmitsPercent(t *testing.T) {
	var buf bytes.Buffer
	p, _ := newTestProgress(&buf, 0, "x")
	p.Add(5_000)
	if strings.Contains(buf.String(), "%") {
		t.Fatalf("percent shown: %q", buf.String())
	}
	if buf.String() != "\rx: 5.0 KB of 0 B" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestProgressNilTerminal(t *testing.T) {
	p := NewProgress(nil, 100, "x")
	p.Add(50)
	p.Done()
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:             "0 B",
		999:           "999 B",
		1000:          "1.0 KB",
		9_949:         "9.9 KB",
		123_000_000:   "123 MB",
		5_500_000:     "5.5 MB",
		3_000_000_000: "3.0 GB",
		290_000_000:   "290 MB",
	}
	for n, want := range cases {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestCountingReaderPassesPayloadAndOnlyDrawsStatus(t *testing.T) {
	payload := strings.Repeat("12% done\r\n34% left 56\n", 500)
	var term bytes.Buffer
	p, c := newTestProgress(&term, int64(len(payload)), "f")
	r := CountingReader(strings.NewReader(payload), p)
	var out bytes.Buffer
	buf := make([]byte, 2200)
	for {
		n, err := r.Read(buf)
		out.Write(buf[:n])
		c.t = c.t.Add(time.Second)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if out.String() != payload {
		t.Fatal("payload altered")
	}
	want := "\rf: 20% 2.2 KB of 11 KB" +
		"\rf: 40% 4.4 KB of 11 KB" +
		"\rf: 60% 6.6 KB of 11 KB" +
		"\rf: 80% 8.8 KB of 11 KB" +
		"\rf: 100% 11 KB of 11 KB"
	if term.String() != want {
		t.Fatalf("terminal output %q, want %q", term.String(), want)
	}
	re := regexp.MustCompile(`^f: \d+% [0-9.]+ [KMG]?B of [0-9.]+ [KMG]?B$`)
	for _, seg := range strings.Split(strings.TrimPrefix(term.String(), "\r"), "\r") {
		if !re.MatchString(seg) {
			t.Errorf("segment %q is not a status line", seg)
		}
	}
}

func TestProgressDoneAfterThrottledAdd(t *testing.T) {
	var buf bytes.Buffer
	p, c := newTestProgress(&buf, 1000, "x")
	p.Add(100)
	c.t = c.t.Add(10 * time.Millisecond)
	p.Add(900)
	if strings.Contains(buf.String(), "100%") {
		t.Fatalf("throttled Add drew: %q", buf.String())
	}
	p.Done()
	if !strings.HasSuffix(buf.String(), "\rx: 100% 1.0 KB of 1.0 KB\n") {
		t.Fatalf("got %q", buf.String())
	}
}

func TestCountingReaderHasNoBulkInterfaces(t *testing.T) {
	r := CountingReader(strings.NewReader("x"), NewProgress(nil, 1, ""))
	if _, ok := r.(io.WriterTo); ok {
		t.Error("must not implement WriteTo")
	}
	if _, ok := r.(io.ReaderFrom); ok {
		t.Error("must not implement ReadFrom")
	}
}
