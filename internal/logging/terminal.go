package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// repeatWindow is how long a line the terminal has shown stays quiet when
// it happens again. A stale browser tab polling a device that is down
// repeats the same failure every two seconds; the terminal says it once,
// and devicedeck.log keeps every occurrence.
const repeatWindow = time.Minute

// terminalValueMax caps one attribute on screen. Driver errors carry whole
// simctl and xcodebuild transcripts; the first line says what went wrong
// and the log file has the rest.
const terminalValueMax = 160

// terminalSeenMax bounds the repeat memory; past it, the memory starts over.
const terminalSeenMax = 256

// terminalHandler writes records for a person watching the terminal: one
// short line each, repeats suppressed. It is not the log — that is the
// run's devicedeck.log, written in full by a separate handler.
type terminalHandler struct {
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
	state *terminalState
}

// terminalState is shared by a handler and every WithAttrs copy of it, so
// repeats are recognised across loggers.
type terminalState struct {
	mu   sync.Mutex
	seen map[string]time.Time
	now  func() time.Time
}

func newTerminalHandler(w io.Writer, level slog.Leveler) *terminalHandler {
	return &terminalHandler{w: w, level: level, state: &terminalState{seen: map[string]time.Time{}, now: time.Now}}
}

// Enabled reports whether the terminal shows records at level.
func (h *terminalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle writes the record as one line unless it repeats one shown within
// repeatWindow.
func (h *terminalHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %-5s %s", r.Time.Format("15:04:05"), r.Level, r.Message)
	write := func(a slog.Attr) bool {
		if a.Key != "took" && a.Key != "remote" {
			fmt.Fprintf(&b, "  %s=%s", a.Key, terminalValue(a.Value.String()))
		}
		return true
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(write)
	line := b.String()
	if h.state.repeated(strings.TrimPrefix(line, r.Time.Format("15:04:05"))) {
		return nil
	}
	_, err := io.WriteString(h.w, line+"\n")
	return err
}

// repeated reports whether key was shown within repeatWindow, and marks it
// shown now otherwise.
func (s *terminalState) repeated(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if at, ok := s.seen[key]; ok && now.Sub(at) < repeatWindow {
		return true
	}
	if len(s.seen) >= terminalSeenMax {
		s.seen = map[string]time.Time{}
	}
	s.seen[key] = now
	return false
}

// terminalValue keeps an attribute to its first line, trimmed to fit.
func terminalValue(v string) string {
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		v = v[:i] + " …"
	}
	if len(v) > terminalValueMax {
		v = v[:terminalValueMax] + "…"
	}
	return v
}

// WithAttrs returns a handler that adds attrs to every line.
func (h *terminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &c
}

// WithGroup is a no-op: terminal lines are flat, and nothing in DeviceDeck
// groups its attributes.
func (h *terminalHandler) WithGroup(string) slog.Handler { return h }
