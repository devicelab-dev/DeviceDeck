package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

// SessionEnder stops everything DeviceDeck runs for one device — its tree
// engine and its video and input sidecars — and powers the device off.
type SessionEnder interface {
	End(ctx context.Context, udid string) error
}

// sessionEnded is the reason a driver gets when the device's session is
// ended under it, so its page can say what happened rather than report a
// refusal.
const sessionEnded = "session ended: the device was shut down"

// SetSessionEnder enables ending a device's session from the console.
func (s *Server) SetSessionEnder(e SessionEnder) { s.ender = e }

// handleEndSession is the console's End session: it drops whatever this
// server holds for the device and shuts the device down, so it stops using
// CPU and memory. A recording in progress is discarded, and whoever is
// driving the device is told the session ended rather than left with a
// socket that quietly stops working.
func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request) {
	if s.ender == nil {
		httpError(w, http.StatusNotImplemented, errors.New("ending a session is not available on this server"))
		return
	}
	udid := r.PathValue("udid")
	if recording, _ := s.capture.Status(udid); recording {
		_, _, _, _ = s.capture.Stop(udid)
	}
	s.inputs.kickHolder(udid, sessionEnded)
	s.launched.forget(udid)
	if s.warm != nil {
		s.warm.forget(udid)
	}
	if err := s.ender.End(r.Context(), udid); err != nil {
		httpError(w, http.StatusBadGateway, fmt.Errorf("end session on %s: %w", udid, err))
		return
	}
	slog.Info("session ended", "udid", udid)
	writeJSON(w, map[string]any{"ok": true})
}

// forget drops a device's warm-up record, so opening it again after its
// session ended starts a new engine instead of reporting the old one ready.
func (w *warmups) forget(udid string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.status, udid)
}

// launchedApps is the set of apps this server has launched, per device.
// The zero value is ready to use.
type launchedApps struct {
	mu sync.Mutex
	m  map[string]map[string]bool
}

func (l *launchedApps) mark(udid, appID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]map[string]bool{}
	}
	if l.m[udid] == nil {
		l.m[udid] = map[string]bool{}
	}
	l.m[udid][appID] = true
}

func (l *launchedApps) has(udid, appID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m[udid][appID]
}

// forget drops a device's launches: after its session ends, opening it
// again launches its build afresh.
func (l *launchedApps) forget(udid string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, udid)
}
