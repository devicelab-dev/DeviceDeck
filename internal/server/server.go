// Package server exposes DeviceDeck's HTTP API: device discovery, input
// injection, screenshots, and the UI tree. All device interaction is
// behind small interfaces so handlers are testable without a Mac's
// simulator stack.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// DeviceLister enumerates booted simulators.
type DeviceLister interface {
	Booted(ctx context.Context) ([]sim.Device, error)
}

// Screenshotter captures a device's screen as PNG bytes.
type Screenshotter interface {
	Screenshot(ctx context.Context, udid string) ([]byte, error)
}

// FrameSender delivers encoded input frames to a device's sidecar.
type FrameSender interface {
	SendFrame(ctx context.Context, udid string, frame []byte) error
}

// TreeSource fetches the UI tree for a device.
type TreeSource interface {
	Snapshot(ctx context.Context, udid, appBundleID string) ([]runner.Node, error)
}

// Server routes the HTTP API onto the injected device backends.
type Server struct {
	devices     DeviceLister
	screenshots Screenshotter
	frames      FrameSender
	trees       TreeSource
	// sleep paces multi-frame gestures; injected so tests run instantly.
	sleep func(time.Duration)
}

// New wires a Server. Any nil dependency disables its endpoints with 503.
func New(devices DeviceLister, screenshots Screenshotter, frames FrameSender, trees TreeSource) *Server {
	return &Server{
		devices:     devices,
		screenshots: screenshots,
		frames:      frames,
		trees:       trees,
		sleep:       time.Sleep,
	}
}

// Handler returns the API routing table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("GET /api/devices/{udid}/screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /api/devices/{udid}/tree", s.handleTree)
	mux.HandleFunc("POST /api/devices/{udid}/tap", s.handleTap)
	mux.HandleFunc("POST /api/devices/{udid}/swipe", s.handleSwipe)
	mux.HandleFunc("POST /api/devices/{udid}/gesture", s.handleGesture)
	mux.HandleFunc("POST /api/devices/{udid}/key", s.handleKey)
	mux.HandleFunc("POST /api/devices/{udid}/button", s.handleButton)
	return mux
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.devices.Booted(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if devices == nil {
		devices = []sim.Device{}
	}
	writeJSON(w, map[string]any{"devices": devices})
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	png, err := s.screenshots.Screenshot(r.Context(), r.PathValue("udid"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.trees.Snapshot(r.Context(), r.PathValue("udid"), r.URL.Query().Get("app"))
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	if nodes == nil {
		nodes = []runner.Node{}
	}
	writeJSON(w, map[string]any{"nodes": nodes})
}

type tapRequest struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	DurationMs int     `json:"durationMs"`
}

func (s *Server) handleTap(w http.ResponseWriter, r *http.Request) {
	var req tapRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !validNorm(req.X) || !validNorm(req.Y) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("x and y must be normalized 0-1"))
		return
	}
	udid := r.PathValue("udid")
	if err := s.frames.SendFrame(r.Context(), udid, input.Touch(input.TouchDown, req.X, req.Y, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	s.sleep(durationOrDefault(req.DurationMs, 60))
	if err := s.frames.SendFrame(r.Context(), udid, input.Touch(input.TouchUp, req.X, req.Y, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("tap"))
}

type swipeRequest struct {
	FromX      float64 `json:"x1"`
	FromY      float64 `json:"y1"`
	ToX        float64 `json:"x2"`
	ToY        float64 `json:"y2"`
	DurationMs int     `json:"durationMs"`
}

func (s *Server) handleSwipe(w http.ResponseWriter, r *http.Request) {
	var req swipeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	for _, v := range []float64{req.FromX, req.FromY, req.ToX, req.ToY} {
		if !validNorm(v) {
			httpError(w, http.StatusBadRequest, fmt.Errorf("coordinates must be normalized 0-1"))
			return
		}
	}
	udid := r.PathValue("udid")
	const steps = 10
	stepPause := durationOrDefault(req.DurationMs, 250) / steps
	if err := s.frames.SendFrame(r.Context(), udid, input.Touch(input.TouchDown, req.FromX, req.FromY, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	for i := 1; i <= steps; i++ {
		s.sleep(stepPause)
		t := float64(i) / steps
		x := req.FromX + (req.ToX-req.FromX)*t
		y := req.FromY + (req.ToY-req.FromY)*t
		if err := s.frames.SendFrame(r.Context(), udid, input.Touch(input.TouchMove, x, y, input.EdgeNone)); err != nil {
			httpError(w, http.StatusBadGateway, err)
			return
		}
	}
	if err := s.frames.SendFrame(r.Context(), udid, input.Touch(input.TouchUp, req.ToX, req.ToY, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("swipe"))
}

// gestureKinds maps API names onto sidecar gesture recipes.
var gestureKinds = map[string]input.Gesture{
	"home":               input.GestureSwipeToHome,
	"appSwitcher":        input.GestureAppSwitcher,
	"notificationCenter": input.GestureNotificationCenter,
	"lockScreen":         input.GestureLockScreen,
}

func (s *Server) handleGesture(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	gesture, ok := gestureKinds[req.Kind]
	if !ok {
		httpError(w, http.StatusBadRequest, fmt.Errorf("unknown gesture %q", req.Kind))
		return
	}
	if err := s.frames.SendFrame(r.Context(), r.PathValue("udid"), input.SystemGesture(gesture)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("gesture"))
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Usage     uint32 `json:"usage"`
		Modifiers byte   `json:"modifiers"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Usage == 0 {
		httpError(w, http.StatusBadRequest, fmt.Errorf("usage is required"))
		return
	}
	if err := s.frames.SendFrame(r.Context(), r.PathValue("udid"), input.Key(req.Modifiers, req.Usage)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("key"))
}

// legacyButtons are the buttons served by the legacy button service.
var legacyButtons = map[string]uint32{"home": 0, "lock": 1}

func (s *Server) handleButton(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Button string `json:"button"`
		Page   uint32 `json:"page"`
		Usage  uint32 `json:"usage"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	var frame []byte
	switch {
	case req.Button != "":
		code, ok := legacyButtons[req.Button]
		if !ok {
			httpError(w, http.StatusBadRequest, fmt.Errorf("unknown button %q", req.Button))
			return
		}
		frame = input.LegacyButton(code)
	case req.Page != 0 && req.Usage != 0:
		frame = input.ButtonPress(req.Page, req.Usage)
	default:
		httpError(w, http.StatusBadRequest, fmt.Errorf("button name or page+usage required"))
		return
	}
	if err := s.frames.SendFrame(r.Context(), r.PathValue("udid"), frame); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("button"))
}

// ---------- helpers ----------

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httpError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return false
	}
	return true
}

func validNorm(v float64) bool { return v >= 0 && v <= 1 }

func durationOrDefault(ms, fallback int) time.Duration {
	if ms <= 0 {
		ms = fallback
	}
	return time.Duration(ms) * time.Millisecond
}

func okResponse(action string) map[string]any {
	return map[string]any{"ok": true, "action": action}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func httpError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
}
