package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// gatedWarmer blocks each warm-up until released, and counts them.
type gatedWarmer struct {
	calls   atomic.Int32
	release chan error
}

func (g *gatedWarmer) Warm(context.Context, string) error {
	g.calls.Add(1)
	return <-g.release
}

func engineState(t *testing.T, srv *Server, method string) EngineStatus {
	t.Helper()
	rec := do(t, srv, method, "/api/devices/AAA/engine", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s engine: %d %s", method, rec.Code, rec.Body)
	}
	var st EngineStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// waitState polls until the warm-up reaches want.
func waitState(t *testing.T, srv *Server, want string) EngineStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st := engineState(t, srv, "GET"); st.State == want {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("engine never reached %q", want)
	return EngineStatus{}
}

func TestEngineWarmUp(t *testing.T) {
	srv := newTestServer(&fakeBackend{})
	g := &gatedWarmer{release: make(chan error, 2)}
	srv.SetEngineWarmer(g, func(udid string) string { return "Building the iOS runner for " + udid })

	if st := engineState(t, srv, "GET"); st.State != "" {
		t.Fatalf("before any warm-up: %+v", st)
	}
	st := engineState(t, srv, "POST")
	if st.State != EngineStarting || st.Detail != "Building the iOS runner for AAA" || st.StartedAt.IsZero() {
		t.Fatalf("started: %+v", st)
	}
	if again := engineState(t, srv, "POST"); again.State != EngineStarting {
		t.Fatalf("second request: %+v", again)
	}
	g.release <- nil
	if ready := waitState(t, srv, EngineReady); ready.Detail != "" {
		t.Errorf("ready still describes the start: %+v", ready)
	}
	engineState(t, srv, "POST") // ready: nothing new starts
	if g.calls.Load() != 1 {
		t.Errorf("warm-ups started = %d, want 1", g.calls.Load())
	}
}

func TestEngineWarmUpFailureIsRetried(t *testing.T) {
	srv := newTestServer(&fakeBackend{})
	g := &gatedWarmer{release: make(chan error, 2)}
	srv.SetEngineWarmer(g, func(string) string { return "" })
	engineState(t, srv, "POST")
	g.release <- errors.New("xcodebuild failed")
	if failed := waitState(t, srv, EngineFailed); failed.Error != "xcodebuild failed" {
		t.Errorf("failed: %+v", failed)
	}
	engineState(t, srv, "POST")
	g.release <- nil
	waitState(t, srv, EngineReady)
	if g.calls.Load() != 2 {
		t.Errorf("a failed warm-up must be retried; calls = %d", g.calls.Load())
	}
}

func TestEngineWithoutWarmer(t *testing.T) {
	srv := newTestServer(&fakeBackend{})
	for _, m := range []string{"POST", "GET"} {
		if st := engineState(t, srv, m); st.State != EngineReady {
			t.Errorf("%s without a warmer = %+v, want ready", m, st)
		}
	}
}

func TestEngineStatusFollowsTheStage(t *testing.T) {
	srv := newTestServer(&fakeBackend{})
	g := &gatedWarmer{release: make(chan error, 1)}
	stage := atomic.Value{}
	stage.Store("Waiting for Android to finish booting")
	srv.SetEngineWarmer(g, func(string) string { return stage.Load().(string) })
	engineState(t, srv, "POST")
	stage.Store("Installing and starting the Android driver")
	if st := engineState(t, srv, "GET"); st.Detail != "Installing and starting the Android driver" {
		t.Errorf("stage not refreshed: %+v", st)
	}
	g.release <- nil
	waitState(t, srv, EngineReady)
}
