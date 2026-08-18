package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	dlios "github.com/devicelab-dev/maestro-runner/pkg/driver/devicelab_ios"
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
	data, err := t.client.Call(ctx, dlios.Command{
		Command:     dlios.CmdSnapshot,
		AppBundleID: appBundleID,
	})
	if err != nil {
		return nil, fmt.Errorf("runner snapshot: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("runner snapshot: empty response")
	}
	return convertNodes(data.Nodes), nil
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
// Coverage waiver: StartEngine, Engine.Snapshot, and Engine.Stop wrap the
// xcodebuild-driven runner lifecycle and only execute against a real booted
// simulator; they are exercised by end-to-end runs, not unit tests.
func StartEngine(ctx context.Context, udid string) (*Engine, error) {
	artifacts, err := dlios.EnsureBuilt(ctx, udid)
	if err != nil {
		return nil, fmt.Errorf("build devicelab-ios-runner: %w", err)
	}
	client, handle, err := dlios.Setup(ctx, dlios.SetupOptions{
		ArtifactsDir:  artifacts,
		SimulatorUDID: udid,
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

// Stop shuts the runner down, gracefully first.
func (e *Engine) Stop(ctx context.Context) error {
	return dlios.GracefulShutdown(ctx, e.client, e.handle)
}

// engineAPI is what Engines needs from an engine; satisfied by *Engine and
// by test fakes so cache behavior is testable without xcodebuild.
type engineAPI interface {
	Snapshot(ctx context.Context, appBundleID string) ([]Node, error)
	Stop(ctx context.Context) error
}

// Engines lazily starts and caches one engine per simulator UDID.
type Engines struct {
	start   func(ctx context.Context, udid string) (engineAPI, error)
	mu      sync.Mutex
	engines map[string]engineAPI
}

// NewEngines builds an engine cache backed by real runner startup.
func NewEngines() *Engines {
	return &Engines{
		start: func(ctx context.Context, udid string) (engineAPI, error) {
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
	e, err := s.engine(ctx, udid)
	if err != nil {
		return nil, err
	}
	nodes, err := e.Snapshot(ctx, appBundleID)
	var runnerErr *dlios.RunnerError
	if err == nil || ctx.Err() != nil || errors.As(err, &runnerErr) {
		return nodes, err
	}
	slog.Warn("tree snapshot failed, restarting engine", "udid", udid, "error", err)
	s.evict(ctx, udid, e)
	if e, err = s.engine(ctx, udid); err != nil {
		return nil, fmt.Errorf("restart tree engine: %w", err)
	}
	return e.Snapshot(ctx, appBundleID)
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
	_ = failed.Stop(context.WithoutCancel(ctx))
}

func (s *Engines) engine(ctx context.Context, udid string) (engineAPI, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.engines[udid]; ok {
		return e, nil
	}
	e, err := s.start(ctx, udid)
	if err != nil {
		return nil, err
	}
	s.engines[udid] = e
	return e, nil
}

// StopAll shuts down every cached engine.
func (s *Engines) StopAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for udid, e := range s.engines {
		_ = e.Stop(ctx)
		delete(s.engines, udid)
	}
}
