// Package server exposes DeviceDeck's HTTP API: device discovery, input
// injection, screenshots, and the UI tree. All device interaction is
// behind small interfaces so handlers are testable without a Mac's
// simulator stack.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/capture"
	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
)

// DeviceLister enumerates devices: All is the whole inventory (booted
// and stopped — the console shows what could be booted, not just what
// runs), Booted the running subset.
type DeviceLister interface {
	All(ctx context.Context) ([]sim.Device, error)
	Booted(ctx context.Context) ([]sim.Device, error)
}

// DeviceBooter starts a stopped device by id.
type DeviceBooter interface {
	Boot(ctx context.Context, id string) error
}

// AppLauncher starts an app fresh on a device — terminating it first so
// a caller can rely on beginning at the app's first screen. ResetApp goes
// further, wiping the app's stored data so the launch begins as a first
// run rather than inheriting a logged-in session from a previous one.
type AppLauncher interface {
	LaunchApp(ctx context.Context, udid, appID string) error
	ResetApp(ctx context.Context, udid, appID string) error
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

// CaptureService records manual sessions as flows.
type CaptureService interface {
	Start(ctx context.Context, udid, appID string) error
	Stop(udid string) (yaml, guard string, steps []capture.Step, err error)
	Assert(udid string, x, y float64) error
	Status(udid string) (recording bool, steps []capture.Step)
	OnFrame(udid string, frame []byte)
}

// Server routes the HTTP API onto the injected device backends.
type Server struct {
	devices     DeviceLister
	boot        DeviceBooter
	launch      AppLauncher
	screenshots Screenshotter
	frames      FrameSender
	trees       TreeSource
	video       VideoSource
	capture     CaptureService
	console     http.Handler
	// inputs enforces one driver per device.
	inputs *inputOwners
	// sleep paces multi-frame gestures; injected so tests run instantly.
	sleep func(time.Duration)
	// Timings for the settle barrier and the launch wait. Zero means the
	// measured defaults; tests shrink them so a wait takes microseconds.
	settle    runner.SettleOptions
	launching runner.LaunchOptions
}

// New wires a Server.
func New(devices DeviceLister, boot DeviceBooter, launch AppLauncher, screenshots Screenshotter, frames FrameSender, trees TreeSource, video VideoSource, cap CaptureService) *Server {
	return &Server{
		devices:     devices,
		boot:        boot,
		launch:      launch,
		screenshots: screenshots,
		frames:      frames,
		trees:       trees,
		video:       video,
		capture:     cap,
		inputs:      newInputOwners(),
		sleep:       time.Sleep,
	}
}

// sendFrame forwards one frame to the device's sidecar and, when a
// recording is active, feeds it to capture — the single choke point both
// REST handlers and the input WebSocket go through.
func (s *Server) sendFrame(ctx context.Context, udid string, frame []byte) error {
	// Capture observes first: it measures gesture timing from frame
	// arrival, and dispatch can block on slow backends (an Android
	// driver click takes ~250ms) — observing afterwards inflated a tap's
	// down→up gap into a long-press.
	s.capture.OnFrame(udid, frame)
	return s.frames.SendFrame(ctx, udid, frame)
}

// SetConsole mounts the browser console at /. Optional — API-only servers
// (and tests) skip it.
func (s *Server) SetConsole(h http.Handler) { s.console = h }

// Handler returns the API routing table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("POST /api/devices/{udid}/boot", s.handleBoot)
	mux.HandleFunc("POST /api/devices/{udid}/app/launch", s.handleLaunchApp)
	mux.HandleFunc("GET /api/devices/{udid}/screenshot", s.handleScreenshot)
	mux.HandleFunc("GET /api/devices/{udid}/tree", s.handleTree)
	mux.HandleFunc("GET /api/devices/{udid}/video", s.handleVideoWS)
	mux.HandleFunc("GET /api/devices/{udid}/input", s.handleInputWS)
	mux.HandleFunc("POST /api/devices/{udid}/tap", s.handleTap)
	mux.HandleFunc("POST /api/devices/{udid}/swipe", s.handleSwipe)
	mux.HandleFunc("POST /api/devices/{udid}/gesture", s.handleGesture)
	mux.HandleFunc("POST /api/devices/{udid}/key", s.handleKey)
	mux.HandleFunc("POST /api/devices/{udid}/button", s.handleButton)
	mux.HandleFunc("POST /api/devices/{udid}/capture/start", s.handleCaptureStart)
	mux.HandleFunc("POST /api/devices/{udid}/capture/stop", s.handleCaptureStop)
	mux.HandleFunc("POST /api/devices/{udid}/capture/assert", s.handleCaptureAssert)
	mux.HandleFunc("GET /api/devices/{udid}/capture", s.handleCaptureStatus)
	if s.console != nil {
		mux.Handle("GET /", s.console)
	}
	return mux
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.devices.All(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	if devices == nil {
		devices = []sim.Device{}
	}
	writeJSON(w, map[string]any{"devices": devices})
}

func (s *Server) handleBoot(w http.ResponseWriter, r *http.Request) {
	if err := s.boot.Boot(r.Context(), r.PathValue("udid")); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "action": "boot"})
}

type launchRequest struct {
	App string `json:"app"`
}

// handleLaunchApp starts an app at a first-run screen by default — data
// wiped, logged out — so a caller begins from the clean slate a new
// automation session expects. ?reset=no resumes the app as it was left.
func (s *Server) handleLaunchApp(w http.ResponseWriter, r *http.Request) {
	var req launchRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.App == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("app bundle id is required"))
		return
	}
	udid := r.PathValue("udid")
	// Launch fresh by default: wipe the app's stored data first so the run
	// begins at a first-run screen — logged out — which is the clean slate
	// a new automation session expects (Appium resets between sessions
	// too). A relaunch alone keeps a logged-in session, which is the
	// surprise that makes web-style tests flaky against a native app. Pass
	// ?reset=no to resume instead, launching onto whatever the last session
	// left — for continuing a flow or inspecting current state.
	if r.URL.Query().Get("reset") != "no" {
		if err := s.launch.ResetApp(r.Context(), udid, req.App); err != nil {
			httpError(w, http.StatusBadGateway, err)
			return
		}
	}
	if err := s.launch.LaunchApp(r.Context(), udid, req.App); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	// Return when the app is taking input, not when the launch command
	// did. A freshly launched app swallows taps for a measured window
	// after its screen is already in the tree, and no client can see
	// that window — so the launch call waits it out for all of them.
	if err := runner.AwaitLaunched(r.Context(), s.snapshotter(udid, req.App), s.launching); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("launch"))
}

// snapshotter binds the tree source to one device and app, which is the
// shape the settle logic wants.
func (s *Server) snapshotter(udid, app string) runner.Snapshotter {
	return func(ctx context.Context) ([]runner.Node, error) {
		return s.trees.Snapshot(ctx, udid, app)
	}
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
	udid := r.PathValue("udid")
	snap := s.snapshotter(udid, r.URL.Query().Get("app"))
	var nodes []runner.Node
	var err error
	// ?after=<interaction hash> turns the poll into a barrier: the
	// response is held until the screen has moved on from that hash and
	// come to rest, or the cap passes. A client that fires this fetch
	// right after an action gets the settled screen back, and a tool
	// that waits on the page's own requests — playwright-mcp does —
	// waits for the device without knowing it.
	if after, held := r.URL.Query()["after"]; held {
		nodes, err = runner.Settle(r.Context(), snap, after[0], s.settle)
	} else {
		nodes, err = snap(r.Context())
	}
	if err != nil {
		// Logged, not just returned: a failed snapshot reaches the page
		// as an empty mirror, and every consumer then reports a missing
		// element instead of a broken engine. Without this the server
		// side of that failure leaves no trace at all, which is how a
		// driver that would not start cost an evening to identify.
		slog.Warn("tree snapshot failed", "udid", udid, "err", err)
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, treePayload(nodes))
}

// treePayload is the tree as every consumer receives it. The hashes
// travel with it so a caller can tell "the screen changed" from "the
// snapshot differs" without re-deriving the exclusions client-side:
// "hash" is the screen, for settledness and the fingerprint;
// "interaction" adds focus, and is what a client hands back as ?after=
// so that moving between fields counts as a change.
func treePayload(nodes []runner.Node) map[string]any {
	if nodes == nil {
		nodes = []runner.Node{}
	}
	return map[string]any{
		"nodes":       nodes,
		"hash":        runner.ScreenHash(nodes),
		"interaction": runner.InteractionHash(nodes),
	}
}

// FirstTree is the settled tree for a device page, ready to inline. It
// waits for quiet alone — there is no earlier screen to have moved on
// from — under the same cap as any other held tree.
func (s *Server) FirstTree(ctx context.Context, udid, app string) ([]byte, error) {
	nodes, err := runner.Settle(ctx, s.snapshotter(udid, app), "", s.settle)
	if err != nil {
		return nil, err
	}
	return json.Marshal(treePayload(nodes))
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
	if err := s.sendFrame(r.Context(), udid, input.Touch(input.TouchDown, req.X, req.Y, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	s.sleep(durationOrDefault(req.DurationMs, 60))
	if err := s.sendFrame(r.Context(), udid, input.Touch(input.TouchUp, req.X, req.Y, input.EdgeNone)); err != nil {
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
	if err := s.sendFrame(r.Context(), udid, input.Touch(input.TouchDown, req.FromX, req.FromY, input.EdgeNone)); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	for i := 1; i <= steps; i++ {
		s.sleep(stepPause)
		t := float64(i) / steps
		x := req.FromX + (req.ToX-req.FromX)*t
		y := req.FromY + (req.ToY-req.FromY)*t
		if err := s.sendFrame(r.Context(), udid, input.Touch(input.TouchMove, x, y, input.EdgeNone)); err != nil {
			httpError(w, http.StatusBadGateway, err)
			return
		}
	}
	if err := s.sendFrame(r.Context(), udid, input.Touch(input.TouchUp, req.ToX, req.ToY, input.EdgeNone)); err != nil {
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
	if err := s.sendFrame(r.Context(), r.PathValue("udid"), input.SystemGesture(gesture)); err != nil {
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
	if err := s.sendFrame(r.Context(), r.PathValue("udid"), input.Key(req.Modifiers, req.Usage)); err != nil {
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
	if err := s.sendFrame(r.Context(), r.PathValue("udid"), frame); err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, okResponse("button"))
}

// ---------- capture ----------

func (s *Server) handleCaptureStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App string `json:"app"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.App == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("app bundle id is required"))
		return
	}
	if err := s.capture.Start(r.Context(), r.PathValue("udid"), req.App); err != nil {
		httpError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, okResponse("capture-start"))
}

func (s *Server) handleCaptureStop(w http.ResponseWriter, r *http.Request) {
	yaml, guard, steps, err := s.capture.Stop(r.PathValue("udid"))
	if err != nil {
		httpError(w, http.StatusConflict, err)
		return
	}
	if steps == nil {
		steps = []capture.Step{}
	}
	writeJSON(w, map[string]any{"ok": true, "yaml": yaml, "guard": guard, "steps": steps})
}

// handleCaptureAssert records an assertion instead of an interaction:
// the point is resolved to an element and asserted visible, and the
// device is never touched.
func (s *Server) handleCaptureAssert(w http.ResponseWriter, r *http.Request) {
	var req tapRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if !validNorm(req.X) || !validNorm(req.Y) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("x and y must be normalized 0-1"))
		return
	}
	if err := s.capture.Assert(r.PathValue("udid"), req.X, req.Y); err != nil {
		httpError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, okResponse("capture-assert"))
}

func (s *Server) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	recording, steps := s.capture.Status(r.PathValue("udid"))
	if steps == nil {
		steps = []capture.Step{}
	}
	writeJSON(w, map[string]any{"recording": recording, "steps": steps})
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
