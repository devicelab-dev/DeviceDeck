package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// The device page performs every action through POST /act and blocks on it
// with a synchronous request. The response is the settled tree after the
// action, which the page applies before its event handler returns.
//
// That is what makes a browser automation tool's click() or fill() finish
// on the device: Chromium acknowledges an input event only after the page's
// handlers for it have run, and Playwright's action resolves on that
// acknowledgement. Measured: a handler blocked 6s → click() 6008ms,
// fill() 6007ms. Anything asynchronous — a socket frame, a fetch — lets the
// tool move on while the device is still acting, and no page-side signal
// (disabled, readonly, an overlay) holds a keyboard action.

// actCap bounds one action end to end: the driver call and the settle that
// follows. It sits under Playwright MCP's default 5s action timeout, so a
// slow device answers with its latest screen rather than failing the tool.
const actCap = 4 * time.Second

// actRequest is one device-page action.
type actRequest struct {
	// ID identifies the action; a repeat of an ID already done returns the
	// same result without acting again, so a retry cannot double-tap.
	ID   string `json:"id"`
	Kind string `json:"kind"` // tap | swipe | fill | key
	// App is the app under test, for the tree read and the iOS runner.
	App string `json:"app"`
	// After is the interaction hash of the screen the action was taken on;
	// the settle waits for the screen to move on from it.
	After string `json:"after"`
	// X, Y: tap point, or swipe start, normalized to the screen.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// ToX, ToY: swipe end, normalized.
	ToX        float64 `json:"toX"`
	ToY        float64 `json:"toY"`
	DurationMs int     `json:"durationMs"`
	// Fill: the field (identifier, only when unique on screen), its centre in
	// the tree's own units, the value it must hold, and what it held.
	Field string  `json:"field"`
	PX    float64 `json:"px"`
	PY    float64 `json:"py"`
	Text  string  `json:"text"`
	Prev  int     `json:"prev"`
	// Key: a HID keyboard usage and modifier bits.
	Usage     uint32 `json:"usage"`
	Modifiers byte   `json:"modifiers"`
}

// handleAct performs one action and answers with the settled tree after it.
func (s *Server) handleAct(w http.ResponseWriter, r *http.Request) {
	var req actRequest
	if !decodeBody(w, r, &req) {
		return
	}
	udid := r.PathValue("udid")
	if done, ok := s.acts.lookup(udid, req.ID); ok {
		writeJSON(w, done)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), actCap)
	defer cancel()
	actErr := s.perform(ctx, udid, req)
	payload, err := s.settleAfter(ctx, udid, req)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	payload["act"] = actOutcome(req.Kind, actErr, ctx.Err())
	s.acts.store(udid, req.ID, payload)
	writeJSON(w, payload)
}

// perform carries out the action and pushes through any input the frame
// sender still buffers, so the settle that follows sees its effect.
func (s *Server) perform(ctx context.Context, udid string, req actRequest) error {
	var err error
	switch req.Kind {
	case "tap":
		err = s.sendTap(ctx, udid, req.X, req.Y, 60)
	case "swipe":
		err = s.sendSwipe(ctx, udid, req.X, req.Y, req.ToX, req.ToY, req.DurationMs)
	case "fill":
		err = s.fill(ctx, udid, fillMessage{
			App: req.App, ID: req.Field, X: req.X, Y: req.Y, PX: req.PX, PY: req.PY, Text: req.Text, Prev: req.Prev,
		})
	case "key":
		err = s.sendFrame(ctx, udid, input.Key(req.Modifiers, req.Usage))
	default:
		return fmt.Errorf("unknown action %q", req.Kind)
	}
	s.flushInput(ctx, udid)
	return err
}

// settleAfter reads the tree once the screen has moved on from the one the
// action was taken on and come to rest — or at the cap, for an action that
// changed nothing. A tree always comes back: the runner seam keeps a
// cancelled context away from the driver (runnerContext), so a read that
// outlives the cap still answers with the latest screen.
func (s *Server) settleAfter(ctx context.Context, udid string, req actRequest) (map[string]any, error) {
	got, err := runner.Settle(ctx, s.snapshotter(udid, req.App), req.After, s.settleFor(udid, req.App))
	if err != nil {
		return nil, err
	}
	return treePayload(got), nil
}

// actOutcome reports how the action went: an error the driver returned,
// and whether the cap cut the action or its settle short.
func actOutcome(kind string, err, deadline error) map[string]any {
	out := map[string]any{"kind": kind, "timedOut": errors.Is(deadline, context.DeadlineExceeded)}
	if err != nil {
		out["error"] = err.Error()
	}
	return out
}

// sendTap presses and releases at x, y (normalized), holding holdMs.
func (s *Server) sendTap(ctx context.Context, udid string, x, y float64, holdMs int) error {
	if err := s.sendFrame(ctx, udid, input.Touch(input.TouchDown, x, y, input.EdgeNone)); err != nil {
		return err
	}
	s.sleep(durationOrDefault(holdMs, 60))
	return s.sendFrame(ctx, udid, input.Touch(input.TouchUp, x, y, input.EdgeNone))
}

// sendSwipe drags from one normalized point to another over durationMs.
func (s *Server) sendSwipe(ctx context.Context, udid string, fromX, fromY, toX, toY float64, durationMs int) error {
	const steps = 10
	stepPause := durationOrDefault(durationMs, 250) / steps
	if err := s.sendFrame(ctx, udid, input.Touch(input.TouchDown, fromX, fromY, input.EdgeNone)); err != nil {
		return err
	}
	for i := 1; i <= steps; i++ {
		s.sleep(stepPause)
		t := float64(i) / steps
		x, y := fromX+(toX-fromX)*t, fromY+(toY-fromY)*t
		if err := s.sendFrame(ctx, udid, input.Touch(input.TouchMove, x, y, input.EdgeNone)); err != nil {
			return err
		}
	}
	return s.sendFrame(ctx, udid, input.Touch(input.TouchUp, toX, toY, input.EdgeNone))
}

// actsRemembered is how many recent action results each device keeps for
// de-duplication: enough for a tool's retry, not a history.
const actsRemembered = 32

// recentActs remembers the last results per device by action ID.
type recentActs struct {
	mu   sync.Mutex
	done map[string][]doneAct
}

type doneAct struct {
	id      string
	payload map[string]any
}

func newRecentActs() *recentActs {
	return &recentActs{done: make(map[string][]doneAct)}
}

// lookup returns the result of an action already performed on udid.
func (a *recentActs) lookup(udid, id string) (map[string]any, bool) {
	if id == "" {
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.done[udid] {
		if d.id == id {
			return d.payload, true
		}
	}
	return nil, false
}

// store records an action's result, forgetting the oldest past the limit.
func (a *recentActs) store(udid, id string, payload map[string]any) {
	if id == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.done[udid] = append(a.done[udid], doneAct{id: id, payload: payload})
	if n := len(a.done[udid]); n > actsRemembered {
		a.done[udid] = a.done[udid][n-actsRemembered:]
	}
}
