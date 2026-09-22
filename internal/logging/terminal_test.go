package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// failingWriter reports every write as failed.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("terminal gone") }

func newTestTerminal(level slog.Level) (*bytes.Buffer, *slog.Logger, *time.Time) {
	var buf bytes.Buffer
	h := newTerminalHandler(&buf, level)
	clock := time.Date(2026, 9, 22, 23, 6, 48, 0, time.UTC)
	h.state.now = func() time.Time { return clock }
	return &buf, slog.New(h), &clock
}

func TestTerminalLineIsShort(t *testing.T) {
	buf, log, _ := newTestTerminal(slog.LevelWarn)
	long := "simctl install: exit status 149 (" + strings.Repeat("x", 300) + ")"
	log.Error("engine start failed", "udid", "AFAC", "took", time.Second, "remote", "127.0.0.1:1",
		"err", errors.New(long+"\nUnable to lookup in current state: Shutting Down"))
	line := buf.String()
	for _, want := range []string{"ERROR engine start failed", "udid=AFAC", "err=simctl install: exit status 149", "…"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks %q", line, want)
		}
	}
	for _, unwanted := range []string{"took=", "remote=", "Shutting Down", "time="} {
		if strings.Contains(line, unwanted) {
			t.Errorf("line %q carries %q, which belongs in the log file only", line, unwanted)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("want exactly one line, got %q", line)
	}
}

func TestTerminalValue(t *testing.T) {
	for in, want := range map[string]string{
		"short":                          "short",
		"first\nsecond":                  "first …",
		strings.Repeat("a", 170):         strings.Repeat("a", terminalValueMax) + "…",
		strings.Repeat("b", 170) + "\nc": strings.Repeat("b", terminalValueMax) + "…",
	} {
		if got := terminalValue(in); got != want {
			t.Errorf("terminalValue(%.20q) = %.30q, want %.30q", in, got, want)
		}
	}
}

func TestTerminalSuppressesRepeats(t *testing.T) {
	buf, log, clock := newTestTerminal(slog.LevelWarn)
	for _, step := range []struct {
		advance time.Duration
		udid    string
		shown   bool
	}{
		{0, "AAA", true},
		{2 * time.Second, "AAA", false}, // a polling tab: same failure, said once
		{2 * time.Second, "BBB", true},  // another device is news
		{repeatWindow, "AAA", true},     // past the window it is shown again
		{time.Second, "AAA", false},
	} {
		*clock = clock.Add(step.advance)
		before := buf.Len()
		log.Warn("engine start failed", "udid", step.udid)
		if shown := buf.Len() > before; shown != step.shown {
			t.Errorf("after %v on %s: shown=%v, want %v", step.advance, step.udid, shown, step.shown)
		}
	}
}

func TestTerminalRepeatMemoryIsBounded(t *testing.T) {
	_, log, _ := newTestTerminal(slog.LevelWarn)
	h := log.Handler().(*terminalHandler)
	for i := 0; i < terminalSeenMax+5; i++ {
		log.Warn("failed", "n", i)
	}
	if n := len(h.state.seen); n > terminalSeenMax {
		t.Errorf("repeat memory holds %d lines, want at most %d", n, terminalSeenMax)
	}
}

func TestTerminalLevelAndAttrs(t *testing.T) {
	buf, log, _ := newTestTerminal(slog.LevelWarn)
	log.Info("engine started")
	if buf.Len() != 0 {
		t.Errorf("info reached a warn terminal: %q", buf.String())
	}
	log.With("udid", "CCC").WithGroup("ignored").Warn("sidecar exited", "code", 1)
	if got := buf.String(); !strings.Contains(got, "WARN  sidecar exited  udid=CCC  code=1") {
		t.Errorf("line = %q", got)
	}
}

func TestTerminalWriteError(t *testing.T) {
	h := newTerminalHandler(failingWriter{}, slog.LevelWarn)
	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), r); err == nil {
		t.Error("a failed terminal write must be reported")
	}
}
