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
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
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

// engineAPI is what Engines needs from an engine; satisfied by *Engine and
// by test fakes so cache behavior is testable without a device.
type engineAPI interface {
	Snapshot(ctx context.Context, appBundleID string) ([]Node, error)
	SnapshotState(ctx context.Context, appBundleID string) (Snapshot, error)
	Stop(ctx context.Context) error
}

// bootWait bounds how long an engine start waits for its simulator to finish
// booting; a cold first boot of a new simulator takes a minute or two.
const bootWait = 3 * time.Minute

// waitBooted holds an iOS engine start until the simulator is fully up; see
// sim.Client.WaitBooted. A variable so tests need no simulator.
var waitBooted = func(ctx context.Context, udid string) error {
	ctx, cancel := context.WithTimeout(ctx, bootWait)
	defer cancel()
	return sim.NewClient().WaitBooted(ctx, udid)
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
			if err := waitBooted(ctx, udid); err != nil {
				return nil, err
			}
			return StartEngine(ctx, udid)
		},
		engines: make(map[string]engineAPI),
	}
}

// Snapshot fetches the UI tree for udid, starting its engine on first use.
// On failure it evicts the engine and retries once on a fresh one: the
// XCUITest agent process can die mid-session (brief §11.4), and a cached
// dead engine would otherwise fail every request until the whole server
// restarts. Two failure classes are surfaced without evicting: a cancelled
// context (a caller going away must not cost a multi-second engine
// restart), and a structured AgentError — the agent answered, so it is
// alive; restarting a live engine over an app-level error (APP_NOT_RUNNING,
// a caught in-agent exception) just burns the startup cost for nothing.
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
	var agentErr *dlios.AgentError
	if err == nil || ctx.Err() != nil || errors.As(err, &agentErr) {
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

// releaser is an engine that can let go of its device while leaving the
// on-device helper running for the next session to re-attach to (the iOS
// agent). Engines without one are stopped.
type releaser interface {
	Release(ctx context.Context)
}

// StopAll lets go of every cached engine as DeviceDeck exits. An engine that
// can be released is — its device's helper stays up, so the next start
// re-attaches in seconds instead of launching it again — and the rest are
// stopped. A failed engine is never released: evict stops it.
func (s *Engines) StopAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for udid, e := range s.engines {
		if r, ok := e.(releaser); ok {
			r.Release(ctx)
			slog.Info("engine released", "udid", udid)
			delete(s.engines, udid)
			continue
		}
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
