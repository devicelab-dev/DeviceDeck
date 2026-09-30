package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// fillingBackend is a tree source whose engine can fill fields, recording
// each request and answering with fillErr.
type fillingBackend struct {
	*fakeBackend
	fills   []runner.FillRequest
	fillErr error
}

func (f *fillingBackend) Fill(_ context.Context, udid string, req runner.FillRequest) error {
	f.fills = append(f.fills, req)
	return f.fillErr
}

// flushingBackend is a frame sender that buffers input and must be flushed.
type flushingBackend struct {
	*fakeBackend
	flushed []string
}

func (f *flushingBackend) Flush(_ context.Context, udid string) error {
	f.flushed = append(f.flushed, udid)
	return errors.New("flush failed")
}

func actBody(t *testing.T, rec interface{ String() string }) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(rec.String()), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.String(), err)
	}
	return out
}

func TestActPerformsEachKindAndReturnsTheTree(t *testing.T) {
	nodes := []runner.Node{{Index: 0, Type: "Button", Label: "Log in"}}
	tests := []struct {
		name       string
		body       string
		wantFrames []byte // first frame type expected, 0 = none
		wantErr    string
	}{
		{"tap", `{"id":"a","kind":"tap","x":0.5,"y":0.5}`, input.Touch(input.TouchDown, 0.5, 0.5, input.EdgeNone), ""},
		{"swipe", `{"id":"b","kind":"swipe","x":0.5,"y":0.8,"toX":0.5,"toY":0.2,"durationMs":100}`, input.Touch(input.TouchDown, 0.5, 0.8, input.EdgeNone), ""},
		{"key", `{"id":"c","kind":"key","usage":40,"modifiers":0}`, input.Key(0, 40), ""},
		{"unknown", `{"id":"d","kind":"wave"}`, nil, `unknown action \"wave\"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeBackend{nodes: nodes}
			rec := do(t, newTestServer(f), "POST", "/api/devices/AAA/act", tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			out := actBody(t, rec.Body)
			if out["hash"] != runner.ScreenHash(nodes) {
				t.Errorf("tree missing from answer: %v", out)
			}
			act := out["act"].(map[string]any)
			if tt.wantErr != "" && !strings.Contains(fmt.Sprint(act["error"]), `unknown action`) {
				t.Errorf("act = %v, want error %s", act, tt.wantErr)
			}
			frames := f.sentFrames()
			if tt.wantFrames == nil && len(frames) != 0 {
				t.Errorf("frames = %d, want none", len(frames))
			}
			if tt.wantFrames != nil && (len(frames) == 0 || string(frames[0]) != string(tt.wantFrames)) {
				t.Errorf("first frame = %x, want %x", frames, tt.wantFrames)
			}
		})
	}
}

func TestActFill(t *testing.T) {
	body := `{"id":"f","kind":"fill","app":"com.example","field":"username-input","x":0.5,"y":0.4,"px":201,"py":350,"text":"devicelab","prev":3}`
	for _, fillErr := range []error{nil, errors.New("TEXT_ENTRY_MISMATCH")} {
		b := &fillingBackend{fakeBackend: &fakeBackend{nodes: []runner.Node{{Index: 0, Type: "TextField"}}}, fillErr: fillErr}
		capture := &fakeCapture{}
		s := newTestServerWithCapture(b.fakeBackend, capture)
		s.trees = b
		out := actBody(t, do(t, s, "POST", "/api/devices/AAA/act", body).Body)
		want := runner.FillRequest{App: "com.example", Identifier: "username-input", X: 201, Y: 350, Text: "devicelab", PrevLen: 3}
		if len(b.fills) != 1 || b.fills[0] != want {
			t.Errorf("fills = %+v, want %+v", b.fills, want)
		}
		if len(capture.fills) != 1 || capture.fills[0] != "devicelab" {
			t.Errorf("capture fills = %v", capture.fills)
		}
		_, failed := out["act"].(map[string]any)["error"]
		if failed != (fillErr != nil) {
			t.Errorf("act = %v with fillErr %v", out["act"], fillErr)
		}
	}
	// A tree source that cannot fill reports it instead of pretending.
	out := actBody(t, do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/act", body).Body)
	if !strings.Contains(fmt.Sprint(out["act"]), "cannot fill") {
		t.Errorf("act = %v", out["act"])
	}
}

// A retried action ID answers from memory: the device is not acted on twice.
func TestActIsNotRepeated(t *testing.T) {
	f := &fakeBackend{nodes: []runner.Node{{Index: 0, Type: "Button"}}}
	s := newTestServer(f)
	body := `{"id":"same","kind":"tap","x":0.5,"y":0.5}`
	first := do(t, s, "POST", "/api/devices/AAA/act", body).Body.String()
	second := do(t, s, "POST", "/api/devices/AAA/act", body).Body.String()
	if first != second || len(f.sentFrames()) != 2 {
		t.Errorf("repeat acted again: frames = %d", len(f.sentFrames()))
	}
	// Without an ID nothing is remembered.
	do(t, s, "POST", "/api/devices/AAA/act", `{"kind":"tap","x":0.5,"y":0.5}`)
	do(t, s, "POST", "/api/devices/AAA/act", `{"kind":"tap","x":0.5,"y":0.5}`)
	if len(f.sentFrames()) != 6 {
		t.Errorf("anonymous actions frames = %d, want 6", len(f.sentFrames()))
	}
}

func TestActErrors(t *testing.T) {
	// Undecodable body.
	if rec := do(t, newTestServer(&fakeBackend{}), "POST", "/api/devices/AAA/act", "{"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body status = %d", rec.Code)
	}
	// The tree cannot be read after the action.
	rec := do(t, newTestServer(&fakeBackend{nodesErr: errors.New("runner down")}), "POST", "/api/devices/AAA/act", `{"id":"x","kind":"tap","x":0.5,"y":0.5}`)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("tree failure status = %d", rec.Code)
	}
	// Input the sender buffers is flushed even when the flush fails.
	fb := &flushingBackend{fakeBackend: &fakeBackend{nodes: []runner.Node{{Index: 0}}}}
	s := newTestServer(fb.fakeBackend)
	s.frames = fb
	do(t, s, "POST", "/api/devices/AAA/act", `{"id":"k","kind":"key","usage":4}`)
	if len(fb.flushed) != 1 {
		t.Errorf("flushed = %v", fb.flushed)
	}
	// A sidecar failure on the first or second frame of a tap or swipe.
	for _, failAfter := range []int{0, 1} {
		for _, body := range []string{`{"kind":"tap","x":0.5,"y":0.5}`, `{"kind":"swipe","x":0.5,"y":0.8,"toX":0.5,"toY":0.2}`} {
			f := &fakeBackend{nodes: []runner.Node{{Index: 0}}, framesErr: map[int]error{0: errors.New("sidecar gone")}[failAfter], failAfter: failAfter}
			out := actBody(t, do(t, newTestServer(f), "POST", "/api/devices/AAA/act", body).Body)
			if _, failed := out["act"].(map[string]any)["error"]; !failed {
				t.Errorf("failAfter=%d %s: act = %v", failAfter, body, out["act"])
			}
		}
	}
}

func TestActOutcome(t *testing.T) {
	out := actOutcome("tap", nil, context.DeadlineExceeded)
	if out["timedOut"] != true || out["error"] != nil {
		t.Errorf("outcome = %v", out)
	}
	out = actOutcome("fill", errors.New("boom"), nil)
	if out["timedOut"] != false || out["error"] != "boom" {
		t.Errorf("outcome = %v", out)
	}
}

func TestRecentActsForgetsTheOldest(t *testing.T) {
	a := newRecentActs()
	for i := 0; i < actsRemembered+5; i++ {
		a.store("AAA", fmt.Sprint(i), map[string]any{"n": i})
	}
	if _, ok := a.lookup("AAA", "0"); ok {
		t.Error("the oldest result should be forgotten")
	}
	if got, ok := a.lookup("AAA", fmt.Sprint(actsRemembered+4)); !ok || got["n"] != actsRemembered+4 {
		t.Errorf("newest = %v, %v", got, ok)
	}
	if _, ok := a.lookup("BBB", "1"); ok {
		t.Error("results are per device")
	}
	a.store("AAA", "", map[string]any{})
	if _, ok := a.lookup("AAA", ""); ok {
		t.Error("an empty ID is never remembered")
	}
}

// The action cap bounds the settle too: a screen that never settles still
// answers, with the cap reported.
func TestActCapReportsTimeout(t *testing.T) {
	if actCap < time.Second || actCap >= 5*time.Second {
		t.Errorf("actCap = %v, want under Playwright MCP's 5s action timeout", actCap)
	}
}

// idlingBackend is a tree source that can wait for the app to go idle.
type idlingBackend struct {
	*fakeBackend
	idled []string
}

func (f *idlingBackend) Idle(_ context.Context, udid, app string) error {
	f.idled = append(f.idled, udid+"/"+app)
	return nil
}

// An action's settle asks the app whether it has finished before a quiet
// screen counts; a tree source without that signal settles on the tree.
func TestActSettleAsksIdle(t *testing.T) {
	b := &idlingBackend{fakeBackend: &fakeBackend{nodes: []runner.Node{{Index: 0, Type: "Button"}}}}
	s := newTestServer(b.fakeBackend)
	s.trees = b
	do(t, s, "POST", "/api/devices/AAA/act", `{"id":"i","kind":"tap","app":"com.example","x":0.5,"y":0.5}`)
	if len(b.idled) == 0 || b.idled[0] != "AAA/com.example" {
		t.Errorf("idled = %v, want the acted-on app", b.idled)
	}
	if opts := newTestServer(&fakeBackend{}).settleFor("AAA", "com.example"); opts.Idle != nil {
		t.Error("a tree source without idle must not get an idle hook")
	}
}
