package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Engine states a warm-up passes through, as the console shows them.
const (
	EngineStarting = "starting"
	EngineReady    = "ready"
	EngineFailed   = "failed"
)

// EngineWarmer starts a device's tree engine ahead of first use.
type EngineWarmer interface {
	Warm(ctx context.Context, udid string) error
}

// EngineStatus is one device's warm-up, for the console's loading screen.
type EngineStatus struct {
	State     string    `json:"state"`            // "", starting, ready or failed
	Detail    string    `json:"detail,omitempty"` // what is happening, for a person
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt,omitzero"`
}

// warmups runs at most one warm-up per device and remembers how it ended.
// Opening a device starts one; the console polls it to say what is going on
// instead of leaving a person to wonder why the first Inspect is slow.
type warmups struct {
	mu     sync.Mutex
	status map[string]EngineStatus
	warmer EngineWarmer
	detail func(udid string) string
	now    func() time.Time
}

// SetEngineWarmer lets opening a device start its engine in the background.
// detail describes the start for a person, such as a first-time runner build.
func (s *Server) SetEngineWarmer(w EngineWarmer, detail func(udid string) string) {
	s.warm = &warmups{status: map[string]EngineStatus{}, warmer: w, detail: detail, now: time.Now}
}

// start begins a warm-up unless one is running or already succeeded, and
// returns the current state. A failed one is retried.
func (w *warmups) start(udid string) EngineStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	if st := w.status[udid]; st.State == EngineStarting || st.State == EngineReady {
		return st
	}
	st := EngineStatus{State: EngineStarting, Detail: w.detail(udid), StartedAt: w.now()}
	w.status[udid] = st
	go w.run(udid)
	return st
}

// run waits for the engine, detached from any request: the warm-up must
// outlive the call that asked for it.
func (w *warmups) run(udid string) {
	err := w.warmer.Warm(context.Background(), udid)
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.status[udid]
	st.State, st.Detail = EngineReady, ""
	if err != nil {
		st.State, st.Error = EngineFailed, err.Error()
		slog.Warn("engine warm-up failed", "udid", udid, "err", err)
	}
	w.status[udid] = st
}

// get reports a warm-up. While it runs, the description is refreshed, so
// the console follows it from stage to stage (waiting for Android to boot,
// then starting its driver).
func (w *warmups) get(udid string) EngineStatus {
	w.mu.Lock()
	st := w.status[udid]
	w.mu.Unlock()
	if st.State == EngineStarting {
		st.Detail = w.detail(udid)
	}
	return st
}

// handleEngineWarm starts (or reports) a device's engine warm-up.
func (s *Server) handleEngineWarm(w http.ResponseWriter, r *http.Request) {
	if s.warm == nil {
		writeJSON(w, EngineStatus{State: EngineReady})
		return
	}
	writeJSON(w, s.warm.start(r.PathValue("udid")))
}

// handleEngineStatus reports a device's warm-up without starting one.
func (s *Server) handleEngineStatus(w http.ResponseWriter, r *http.Request) {
	if s.warm == nil {
		writeJSON(w, EngineStatus{State: EngineReady})
		return
	}
	writeJSON(w, s.warm.get(r.PathValue("udid")))
}
