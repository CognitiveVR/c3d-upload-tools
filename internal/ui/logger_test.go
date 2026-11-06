package ui

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Options{Verbose: true})
	l.Info("hello %s", "world")
	l.Warn("w")
	l.Error("e")
	l.Debug("d")
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	for i, level := range []string{"INFO", "WARN", "ERROR", "DEBUG"} {
		re := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] \[` + level + `\] \S`)
		if !re.MatchString(lines[i]) {
			t.Errorf("line %d %q does not match for %s", i, lines[i], level)
		}
	}
	if !strings.HasSuffix(lines[0], "] hello world") {
		t.Errorf("message not formatted: %q", lines[0])
	}
}

func TestDebugSilentWithoutVerbose(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{}).Debug("hidden")
	if buf.Len() != 0 {
		t.Fatalf("debug wrote %q", buf.String())
	}
}

func TestQuietSilencesInfoAndDebugOnly(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Options{Quiet: true, Verbose: true})
	l.Info("i")
	l.Debug("d")
	if buf.Len() != 0 {
		t.Fatalf("quiet wrote %q", buf.String())
	}
	l.Warn("w")
	l.Error("e")
	out := buf.String()
	if !strings.Contains(out, "[WARN] w") || !strings.Contains(out, "[ERROR] e") {
		t.Fatalf("warn/error missing: %q", out)
	}
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("line %q: %v", line, err)
	}
	return rec
}

func TestJSONRecords(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Options{JSON: true, Verbose: true, Color: true})
	l.Info("hi %d", 1)
	l.Debug("dbg")
	l.ErrorRecord("not_found", "scene missing", "check the id", 404)
	l.ErrorRecord("usage", "bad flag", "", 0)
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	rec := decode(t, lines[0])
	if rec["level"] != "info" || rec["message"] != "hi 1" || len(rec) != 2 {
		t.Errorf("info record: %v", rec)
	}
	if rec := decode(t, lines[1]); rec["level"] != "debug" {
		t.Errorf("debug record: %v", rec)
	}
	rec = decode(t, lines[2])
	if rec["level"] != "error" || rec["error"] != "not_found" || rec["message"] != "scene missing" ||
		rec["hint"] != "check the id" || rec["status_code"] != float64(404) {
		t.Errorf("error record: %v", rec)
	}
	rec = decode(t, lines[3])
	if _, ok := rec["hint"]; ok {
		t.Errorf("empty hint not omitted: %v", rec)
	}
	if _, ok := rec["status_code"]; ok {
		t.Errorf("zero status_code not omitted: %v", rec)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Error("color in JSON mode")
	}
}

func TestJSONQuietStillWritesWarnAndError(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Options{JSON: true, Quiet: true})
	l.Info("i")
	l.Warn("w")
	if rec := decode(t, strings.TrimSpace(buf.String())); rec["level"] != "warn" {
		t.Fatalf("got %v", rec)
	}
}

func TestErrorRecordText(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, Options{}).ErrorRecord("auth", "key expired", "rotate it", 401)
	out := buf.String()
	if !strings.Contains(out, "[ERROR] key expired") || !strings.Contains(out, "rotate it") {
		t.Fatalf("got %q", out)
	}
}

func TestColorOffForNonTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var buf bytes.Buffer
	New(&buf, Options{Color: true}).Info("x")
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("color on non-terminal writer: %q", buf.String())
	}
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	New(f, Options{Color: true}).Info("x")
	b, _ := os.ReadFile(f.Name())
	if strings.Contains(string(b), "\x1b") {
		t.Fatalf("color on regular file: %q", b)
	}
}

func TestShouldColorRespectsOptionsAndEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if shouldColor(Options{Color: true}, os.Stderr) {
		t.Error("NO_COLOR set must disable color")
	}
	t.Setenv("NO_COLOR", "")
	if shouldColor(Options{Color: false}, os.Stderr) {
		t.Error("Color false must disable color")
	}
	if shouldColor(Options{Color: true, JSON: true}, os.Stderr) {
		t.Error("JSON must disable color")
	}
}

func TestColorOnTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		t.Skip("no controlling terminal")
	}
	defer tty.Close()
	if !shouldColor(Options{Color: true}, tty) {
		t.Error("expected color on a terminal")
	}
}

func TestIsTerminal(t *testing.T) {
	if IsTerminal(nil) {
		t.Error("nil file")
	}
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("regular file")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if IsTerminal(null) {
		t.Error("null device")
	}
}

func TestConcurrentWritesDoNotInterleave(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, Options{JSON: true})
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Info("message")
		}()
	}
	wg.Wait()
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("interleaved line %q", line)
		}
	}
}
