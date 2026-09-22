package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/devicelab-dev/DeviceDeck/internal/apps"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// warmerFunc adapts a function to EngineWarmer.
type warmerFunc func(ctx context.Context, udid string) error

func (f warmerFunc) Warm(ctx context.Context, udid string) error { return f(ctx, udid) }

// fakeEnder records which devices had their session ended.
type fakeEnder struct {
	ended []string
	err   error
}

func (f *fakeEnder) End(_ context.Context, udid string) error {
	f.ended = append(f.ended, udid)
	return f.err
}

func TestEndSession(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ender     *fakeEnder
		recording bool
		status    int
	}{
		{"shuts the device down", &fakeEnder{}, false, http.StatusOK},
		{"discards a recording in progress", &fakeEnder{}, true, http.StatusOK},
		{"reports a failed shutdown", &fakeEnder{err: errors.New("simctl shutdown: exit 1")}, false, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeCapture{recording: tc.recording}
			s := newTestServerWithCapture(&fakeBackend{}, c)
			s.SetSessionEnder(tc.ender)
			s.SetEngineWarmer(warmerFunc(func(context.Context, string) error { return nil }), func(string) string { return "" })
			s.warm.status["AAA"] = EngineStatus{State: EngineReady}
			rec := do(t, s, "POST", "/api/devices/AAA/shutdown", "")
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if len(tc.ender.ended) != 1 || tc.ender.ended[0] != "AAA" {
				t.Errorf("ended = %v, want [AAA]", tc.ender.ended)
			}
			if recording, _ := c.Status("AAA"); recording {
				t.Error("recording still running after the session ended")
			}
			if st := s.warm.get("AAA"); st.State != "" {
				t.Errorf("warm-up still %q: reopening would report the stopped engine ready", st.State)
			}
		})
	}
}

func TestEndSessionUnavailable(t *testing.T) {
	s := newTestServer(&fakeBackend{})
	if rec := do(t, s, "POST", "/api/devices/AAA/shutdown", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501 without an ender", rec.Code)
	}
}

// Whoever is driving the device is told the session ended, and the device
// is free for the next driver.
func TestEndSessionTellsTheDriver(t *testing.T) {
	backend := &fakeBackend{}
	s := New(backend, backend, backend, backend, backend, backend, &fakeVideo{frames: make(chan []byte)}, &fakeCapture{})
	s.SetSessionEnder(&fakeEnder{})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsAddr(srv, "/api/devices/AAA/input"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	// The claim is taken after the upgrade; wait until it is held.
	for s.inputs.heldBy("AAA") == "" {
		time.Sleep(5 * time.Millisecond)
	}
	resp, err := http.Post(srv.URL+"/api/devices/AAA/shutdown", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	_, _, readErr := conn.Read(ctx)
	if !strings.Contains(readErr.Error(), sessionEnded) {
		t.Errorf("driver was not told the session ended: %v", readErr)
	}
	if got := s.inputs.heldBy("AAA"); got != "" {
		t.Errorf("device still held by %q", got)
	}
}

// Opening a device launches its build once: the listing says which builds
// this server has launched there, and ending the session forgets them.
func TestLaunchedAppsAreRemembered(t *testing.T) {
	f := &fakeBackend{nodes: []runner.Node{{Type: "Button", Label: "Sign In", Enabled: true}}}
	s := newTestServer(f)
	s.SetApps(&fakeApps{offer: []apps.Listed{{App: apps.App{ID: "com.example"}}, {App: apps.App{ID: "com.other"}}}})
	s.SetSessionEnder(&fakeEnder{})
	launched := func() string {
		body := do(t, s, "GET", "/api/devices/AAA/apps", "").Body.String()
		return fmt.Sprint(strings.Count(body, `"launched":true`))
	}
	if got := launched(); got != "0" {
		t.Fatalf("launched before any launch: %s", got)
	}
	if rec := do(t, s, "POST", "/api/devices/AAA/app/launch", `{"app":"com.example"}`); rec.Code != http.StatusOK {
		t.Fatalf("launch: %d %s", rec.Code, rec.Body)
	}
	if got := launched(); got != "1" {
		t.Errorf("after launching com.example, %s builds marked launched, want 1", got)
	}
	if other := do(t, s, "GET", "/api/devices/BBB/apps", "").Body.String(); strings.Contains(other, `"launched":true`) {
		t.Errorf("a launch on AAA marked BBB: %s", other)
	}
	do(t, s, "POST", "/api/devices/AAA/shutdown", "")
	if got := launched(); got != "0" {
		t.Errorf("after End session, %s builds still marked launched", got)
	}
}
