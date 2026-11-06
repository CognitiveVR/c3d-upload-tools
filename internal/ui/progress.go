package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const progressRedrawInterval = 100 * time.Millisecond

// Progress redraws a single status line on a terminal writer. It never sees
// payload bytes, only counts.
type Progress struct {
	mu       sync.Mutex
	terminal io.Writer
	total    int64
	done     int64
	label    string
	lastDraw time.Time
	drawn    bool
	now      func() time.Time
}

// NewProgress returns a Progress that draws to terminal; a nil terminal makes
// Add and Done no-ops.
func NewProgress(terminal io.Writer, total int64, label string) *Progress {
	return &Progress{terminal: terminal, total: total, label: label, now: time.Now}
}

// SetTotal replaces the total the next line is drawn against; it does not
// redraw.
func (p *Progress) SetTotal(total int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = total
}

// Add records n more bytes transferred and redraws at most every 100
// milliseconds.
func (p *Progress) Add(n int64) {
	if p == nil || p.terminal == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done += n
	now := p.now()
	if p.drawn && now.Sub(p.lastDraw) < progressRedrawInterval {
		return
	}
	p.drawn = true
	p.lastDraw = now
	_, _ = io.WriteString(p.terminal, "\r"+p.line(p.done))
}

// Done writes the final status line, at 100% when the total is known.
func (p *Progress) Done() {
	if p == nil || p.terminal == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	done := p.done
	if p.total > 0 {
		done = p.total
	}
	_, _ = io.WriteString(p.terminal, "\r"+p.line(done)+"\n")
	p.drawn = false
}

// Abort ends a drawn status line with a newline, so the next log line starts
// on its own line. Without an unfinished line it does nothing, so it is safe
// to defer next to Done.
func (p *Progress) Abort() {
	if p == nil || p.terminal == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.drawn {
		_, _ = io.WriteString(p.terminal, "\n")
		p.drawn = false
	}
}

func (p *Progress) line(done int64) string {
	if p.total <= 0 {
		return fmt.Sprintf("%s: %s of %s", p.label, formatBytes(done), formatBytes(p.total))
	}
	percent := done * 100 / p.total
	if percent > 100 {
		percent = 100
	}
	return fmt.Sprintf("%s: %d%% %s of %s", p.label, percent, formatBytes(done), formatBytes(p.total))
}

func formatBytes(n int64) string {
	units := [...]string{"KB", "MB", "GB"}
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	unit := ""
	for _, u := range units {
		value /= 1000
		unit = u
		if value < 999.5 {
			break
		}
	}
	if value < 9.95 {
		return fmt.Sprintf("%.1f %s", value, unit)
	}
	return fmt.Sprintf("%.0f %s", value, unit)
}

type countingReader struct {
	r io.Reader
	p *Progress
}

// CountingReader wraps r so every byte read advances p. It neither buffers nor
// copies payload.
func CountingReader(r io.Reader, p *Progress) io.Reader {
	return &countingReader{r: r, p: p}
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	if n > 0 {
		c.p.Add(int64(n))
	}
	return n, err
}
