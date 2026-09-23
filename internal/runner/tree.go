package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

// Node is one element of the UI tree in DeviceDeck's own shape. The runner
// sends a flat slice; parent/child structure is recovered via ParentIndex.
type Node struct {
	Index       int    `json:"index"`
	Type        string `json:"type"`
	Label       string `json:"label,omitempty"`
	Identifier  string `json:"identifier,omitempty"`
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Frame       Rect   `json:"frame"`
	Enabled     bool   `json:"enabled"`
	Focused     bool   `json:"focused,omitempty"`
	Selected    bool   `json:"selected,omitempty"`
	Hittable    bool   `json:"hittable"`
	Depth       int    `json:"depth"`
	ParentIndex *int   `json:"parentIndex,omitempty"`
	// ClassName is the platform's raw widget class, carried only on
	// Android (e.g. "com.facebook.react.views.view.ReactViewGroup"). It is
	// what tells a React Native screen from a Flutter or Compose one after
	// Type has been normalised to the shared vocabulary; empty on iOS,
	// where Type already holds the raw XCUIElementType.
	ClassName string `json:"className,omitempty"`
}

// Rect is an element's bounds in device points.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// TreeClient fetches UI snapshots from a running devicelab-ios-runner.
type TreeClient struct {
	client *dlios.Client
}

// NewTreeClient targets a runner listening on host:port.
func NewTreeClient(host string, port int) *TreeClient {
	return &TreeClient{client: dlios.NewClient(host, port)}
}

// Snapshot returns the full UI tree for appBundleID (empty = foreground app,
// runner-side default).
func (t *TreeClient) Snapshot(ctx context.Context, appBundleID string) ([]Node, error) {
	snap, err := t.SnapshotState(ctx, appBundleID)
	return snap.Nodes, err
}

// SnapshotState is Snapshot plus the app's lifecycle state, read from the
// same dump. A snapshot is not an interaction command, so the runner does
// not activate the app first: AppState is its true frontmost/background
// state, which the node tree cannot report reliably.
func (t *TreeClient) SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error) {
	data, err := t.client.Call(runnerContext(ctx), dlios.Command{
		Command:     dlios.CmdSnapshot,
		AppBundleID: appBundleID,
	})
	if snap, ok := notFrontmost(data, err); ok {
		return snap, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("runner snapshot: %w", err)
	}
	if data == nil {
		return Snapshot{}, fmt.Errorf("runner snapshot: empty response")
	}
	// Culled here rather than in the browser: the console, the device
	// page and Flow Capture all read this tree, and a flow recorded
	// against a screen nobody can see is the failure that survives to
	// real hardware.
	return Snapshot{Nodes: OnScreen(convertNodes(data.Nodes)), AppState: data.AppState}, nil
}

// notFrontmost reads a snapshot the runner declined because the app is not
// in front. The runner no longer brings an app forward to read it — that
// hid the very "app left the screen" state DeviceDeck reports — so a
// backgrounded app answers SNAPSHOT_FAILED with its state: no tree, and
// the state says why.
func notFrontmost(data *dlios.ResponseData, err error) (Snapshot, bool) {
	var runnerErr *dlios.RunnerError
	if !errors.As(err, &runnerErr) || runnerErr.Code != dlios.ErrSnapshotFailed || data == nil {
		return Snapshot{}, false
	}
	if data.AppState == "" || data.AppState == appForeground {
		return Snapshot{}, false
	}
	return Snapshot{AppState: data.AppState}, true
}

// appForeground is the runner's state for an app in front, taking input.
const appForeground = "runningForeground"

// idleCapMs bounds one idle wait. Measured: iOS keeps a pushed-away screen
// in the tree ~430ms after the push animation stops; an app that never
// idles (a spinner, a looping animation) costs at most this.
const idleCapMs = 1000

// Idle waits, up to idleCapMs, for the app to finish its work — run loop
// idle and animations done, XCTest's own quiescence. The tree alone cannot
// tell a finished transition from one that has stopped moving but not yet
// removed the old screen.
func (t *TreeClient) Idle(ctx context.Context, appBundleID string) error {
	_, err := t.client.Idle(runnerContext(ctx), appBundleID, idleCapMs)
	return err
}

// runnerContext keeps a caller's cancellation away from the runner. The
// device page cancels its in-flight tree request whenever a new action
// starts, and maestro-runner's client reports a cancelled request as a
// transport failure — which its supervisor answers by killing the (healthy)
// runner and relaunching it, taking down whatever command was running on it:
// measured as every fill failing mid-command. A call started here runs to
// completion; a result nobody waits for any more is simply dropped. The
// client's own timeout still bounds it.
func runnerContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

func convertNodes(in []dlios.SnapshotNode) []Node {
	out := make([]Node, len(in))
	for i, n := range in {
		out[i] = Node{
			Index:       n.Index,
			Type:        n.Type,
			Label:       n.Label,
			Identifier:  n.Identifier,
			Value:       n.Value,
			Placeholder: n.PlaceholderValue,
			Frame:       Rect{X: n.Rect.X, Y: n.Rect.Y, Width: n.Rect.Width, Height: n.Rect.Height},
			Enabled:     n.Enabled,
			Focused:     n.Focused,
			Selected:    n.Selected,
			Hittable:    n.Hittable,
			Depth:       n.Depth,
			ParentIndex: n.ParentIndex,
		}
	}
	return out
}

// Engine owns the XCUITest runner lifecycle for one simulator: EnsureBuilt
// compiles/caches the runner app, Setup boots it via xcodebuild and waits
// for readiness. Startup costs seconds — Engines are cached per UDID and
// live until Stop.
type Engine struct {
	tree   *TreeClient
	handle *dlios.RunnerHandle
	client *dlios.Client
}

// StartEngine builds (first run only) and launches the runner on udid.
//
// Coverage waiver: StartEngine, Engine.Snapshot, Engine.SnapshotState, and
// Engine.Stop wrap the xcodebuild-driven runner lifecycle and only execute
// against a real booted simulator; they are exercised by end-to-end runs,
// not unit tests.
func StartEngine(ctx context.Context, udid string) (*Engine, error) {
	// Embedded, not installed: the runner source ships inside the pinned
	// maestro-runner module, so the runner DeviceDeck builds always
	// matches the client it compiled against — no maestro-runner
	// installation on the machine, no protocol skew.
	artifacts, err := dlios.EnsureBuiltEmbedded(ctx, udid)
	if err != nil {
		return nil, fmt.Errorf("build devicelab-ios-runner: %w", err)
	}
	client, handle, err := dlios.Setup(ctx, dlios.SetupOptions{
		ArtifactsDir:  artifacts,
		SimulatorUDID: udid,
		// A failed start must never reboot the simulator: it is the user's,
		// watched in the console, with the app state they are testing.
		NoSimulatorReset: true,
	})
	if err != nil {
		return nil, fmt.Errorf("start devicelab-ios-runner: %w", err)
	}
	return &Engine{tree: &TreeClient{client: client}, handle: handle, client: client}, nil
}

// Snapshot fetches the UI tree via the engine's client.
func (e *Engine) Snapshot(ctx context.Context, appBundleID string) ([]Node, error) {
	return e.tree.Snapshot(ctx, appBundleID)
}

// SnapshotState fetches the UI tree and the app's lifecycle state.
func (e *Engine) SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error) {
	return e.tree.SnapshotState(ctx, appBundleID)
}

// Idle waits for the app to finish its work; see TreeClient.Idle.
func (e *Engine) Idle(ctx context.Context, appBundleID string) error {
	return e.tree.Idle(ctx, appBundleID)
}

// Stop shuts the runner down, gracefully first.
func (e *Engine) Stop(ctx context.Context) error {
	return dlios.GracefulShutdown(ctx, e.client, e.handle)
}

// engineAPI is what Engines needs from an engine; satisfied by *Engine and
// by test fakes so cache behavior is testable without xcodebuild.
type engineAPI interface {
	Snapshot(ctx context.Context, appBundleID string) ([]Node, error)
	SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error)
	Stop(ctx context.Context) error
}

// Engines lazily starts and caches one engine per simulator UDID.
type Engines struct {
	start   func(ctx context.Context, udid string) (engineAPI, error)
	mu      sync.Mutex
	engines map[string]engineAPI
}

// NewEngines builds an engine cache backed by real runner startup,
// routing each device to its platform's engine.
func NewEngines() *Engines {
	return &Engines{
		start: func(ctx context.Context, udid string) (engineAPI, error) {
			if platform.IsAndroidSerial(udid) {
				return StartAndroidEngine(ctx, udid)
			}
			return StartEngine(ctx, udid)
		},
		engines: make(map[string]engineAPI),
	}
}

// Snapshot fetches the UI tree for udid, starting its engine on first use.
// On failure it evicts the engine and retries once on a fresh one: the
// XCUITest runner process can die mid-session (brief §11.4), and a cached
// dead engine would otherwise fail every request until the whole server
// restarts. Two failure classes are surfaced without evicting: a cancelled
// context (a caller going away must not cost a multi-second engine
// restart), and a structured RunnerError — the runner answered, so it is
// alive; restarting a live engine over an app-level error (APP_NOT_RUNNING,
// a caught in-runner exception) just burns the startup cost for nothing.
func (s *Engines) Snapshot(ctx context.Context, udid, appBundleID string) ([]Node, error) {
	snap, err := s.SnapshotState(ctx, udid, appBundleID)
	return snap.Nodes, err
}

// SnapshotState is Snapshot carrying the app's lifecycle state, with the
// same evict-and-retry behavior.
func (s *Engines) SnapshotState(ctx context.Context, udid, appBundleID string) (Snapshot, error) {
	e, err := s.engine(ctx, udid)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := e.SnapshotState(ctx, appBundleID)
	var runnerErr *dlios.RunnerError
	if err == nil || ctx.Err() != nil || errors.As(err, &runnerErr) {
		return snap, err
	}
	slog.Warn("tree snapshot failed, restarting engine", "udid", udid, "error", err)
	s.evict(ctx, udid, e)
	if e, err = s.engine(ctx, udid); err != nil {
		return Snapshot{}, fmt.Errorf("restart tree engine: %w", err)
	}
	return e.SnapshotState(ctx, appBundleID)
}

// idler is an engine that can wait for its app to go idle (iOS).
type idler interface {
	Idle(ctx context.Context, appBundleID string) error
}

// Idle waits for the app on udid to finish its work, where the platform
// can tell; elsewhere it returns at once.
func (s *Engines) Idle(ctx context.Context, udid, appBundleID string) error {
	e, err := s.engine(ctx, udid)
	if err != nil {
		return err
	}
	if i, ok := e.(idler); ok {
		return i.Idle(ctx, appBundleID)
	}
	return nil
}

// evict drops failed from the cache and stops it — unless a concurrent
// caller already replaced it. The identity check keeps a straggler holding
// the dead engine from tearing down its healthy replacement.
func (s *Engines) evict(ctx context.Context, udid string, failed engineAPI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engines[udid] != failed {
		return
	}
	delete(s.engines, udid)
	if err := failed.Stop(context.WithoutCancel(ctx)); err != nil {
		slog.Warn("engine stop failed", "udid", udid, "err", err)
	}
	slog.Info("engine evicted", "udid", udid)
}

func (s *Engines) engine(ctx context.Context, udid string) (engineAPI, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.engines[udid]; ok {
		return e, nil
	}
	slog.Info("engine starting", "udid", udid)
	began := time.Now()
	e, err := s.start(ctx, udid)
	if err != nil {
		level := slog.LevelError
		if notBooted(err) {
			level = slog.LevelDebug
		}
		slog.Log(ctx, level, "engine start failed", "udid", udid, "took", time.Since(began), "err", err)
		return nil, err
	}
	slog.Info("engine started", "udid", udid, "took", time.Since(began))
	s.engines[udid] = e
	return e, nil
}

// Warm starts udid's engine now, or finds it already running, so the first
// tree read, Inspect or recording does not pay the startup. It returns when
// the engine answers, or with the reason it could not start.
func (s *Engines) Warm(ctx context.Context, udid string) error {
	_, err := s.engine(ctx, udid)
	return err
}

// AndroidInjector returns the input injector for an Android serial,
// starting (or reusing) its engine — input rides the same session as
// the tree, so the first tap on a cold device pays engine startup once.
func (s *Engines) AndroidInjector(ctx context.Context, udid string) (*AndroidEngine, error) {
	e, err := s.engine(ctx, udid)
	if err != nil {
		return nil, err
	}
	ae, ok := e.(*AndroidEngine)
	if !ok {
		return nil, fmt.Errorf("device %s is not an Android engine", udid)
	}
	return ae, nil
}

// StopAll shuts down every cached engine.
func (s *Engines) StopAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for udid := range s.engines {
		s.stopLocked(ctx, udid)
	}
}

// Stop shuts down udid's engine, if one is running, so the next use starts
// a new one. Ending a device's session calls it before powering the device
// off.
func (s *Engines) Stop(ctx context.Context, udid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked(ctx, udid)
}

func (s *Engines) stopLocked(ctx context.Context, udid string) {
	e, ok := s.engines[udid]
	if !ok {
		return
	}
	if err := e.Stop(ctx); err != nil {
		slog.Warn("engine stop failed", "udid", udid, "err", err)
	}
	slog.Info("engine stopped", "udid", udid)
	delete(s.engines, udid)
}

// ActiveUDIDs lists the devices DeviceDeck has an engine on — the ones it
// actually drove. Captured before StopAll (which clears the map) so exit
// cleanup can shut those devices down and leave untouched ones alone.
func (s *Engines) ActiveUDIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	udids := make([]string, 0, len(s.engines))
	for udid := range s.engines {
		udids = append(udids, udid)
	}
	return udids
}

// notBootedStates are the CoreSimulator states simctl names when it refuses
// to act on a simulator that is not running.
var notBootedStates = []string{"current state: Shutdown", "current state: Shutting Down"}

// notBooted reports whether an engine start failed only because the
// simulator is not running. A console tab left open asks for its device's
// engine the moment the server starts, before anyone has booted it; that
// is expected, so it is logged at debug rather than as an error.
func notBooted(err error) bool {
	for _, state := range notBootedStates {
		if strings.Contains(err.Error(), state) {
			return true
		}
	}
	return false
}
