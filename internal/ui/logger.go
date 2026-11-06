// Package ui writes human diagnostics and progress to stderr.
package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const timestampLayout = "2006-01-02 15:04:05"

// Options selects the logger's output format and verbosity.
type Options struct {
	JSON    bool
	Verbose bool
	Quiet   bool
	Color   bool
}

type level int

const (
	levelInfo level = iota
	levelWarn
	levelError
	levelDebug
)

var levelTags = [...]string{"INFO", "WARN", "ERROR", "DEBUG"}
var levelNames = [...]string{"info", "warn", "error", "debug"}
var levelColors = [...]string{"32", "33", "31", "36"}

// Logger is safe for concurrent use.
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	opts  Options
	color bool
	now   func() time.Time
}

// New returns a Logger writing to w.
func New(w io.Writer, opts Options) *Logger {
	return &Logger{w: w, opts: opts, color: shouldColor(opts, w), now: time.Now}
}

func shouldColor(opts Options, w io.Writer) bool {
	if !opts.Color || opts.JSON || os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && IsTerminal(f)
}

func (l *Logger) Info(format string, args ...any)  { l.log(levelInfo, format, args) }
func (l *Logger) Warn(format string, args ...any)  { l.log(levelWarn, format, args) }
func (l *Logger) Error(format string, args ...any) { l.log(levelError, format, args) }
func (l *Logger) Debug(format string, args ...any) { l.log(levelDebug, format, args) }

func (l *Logger) log(lv level, format string, args []any) {
	if lv == levelDebug && !l.opts.Verbose {
		return
	}
	if l.opts.Quiet && (lv == levelInfo || lv == levelDebug) {
		return
	}
	l.emit(lv, fmt.Sprintf(format, args...), nil)
}

type jsonRecord struct {
	Level      string `json:"level"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
	Hint       string `json:"hint,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

// ErrorRecord logs an error with a machine-readable code, an optional hint and
// an optional HTTP status code (zero means none).
func (l *Logger) ErrorRecord(code, message, hint string, statusCode int) {
	text := message
	if hint != "" {
		text = fmt.Sprintf("%s (hint: %s)", message, hint)
	}
	l.emit(levelError, text, &jsonRecord{Error: code, Message: message, Hint: hint, StatusCode: statusCode})
}

func (l *Logger) emit(lv level, text string, rec *jsonRecord) {
	var line []byte
	if l.opts.JSON {
		if rec == nil {
			rec = &jsonRecord{Message: text}
		}
		rec.Level = levelNames[lv]
		line, _ = json.Marshal(rec)
		line = append(line, '\n')
	} else {
		tag := "[" + levelTags[lv] + "]"
		if l.color {
			tag = "[\x1b[" + levelColors[lv] + "m" + levelTags[lv] + "\x1b[0m]"
		}
		line = []byte("[" + l.now().Format(timestampLayout) + "] " + tag + " " + printableLines(text) + "\n")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(line)
}

// printableLines applies Printable to each line, keeping the newlines that
// separate them.
func printableLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = Printable(line)
	}
	return strings.Join(lines, "\n")
}
