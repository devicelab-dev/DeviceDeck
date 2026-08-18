package capture

import (
	"context"
	"fmt"
	"sync"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// TreeSource fetches UI trees — satisfied by runner.Engines.
type TreeSource interface {
	Snapshot(ctx context.Context, udid, appBundleID string) ([]runner.Node, error)
}

// Service manages at most one active Recorder per device.
type Service struct {
	trees TreeSource

	mu        sync.Mutex
	recorders map[string]*Recorder
}

// NewService builds a Service resolving selectors through trees.
func NewService(trees TreeSource) *Service {
	return &Service{trees: trees, recorders: make(map[string]*Recorder)}
}

// Start begins recording udid's session against appID. Fails if a
// recording is already active for the device.
func (s *Service) Start(ctx context.Context, udid, appID string) error {
	s.mu.Lock()
	if _, active := s.recorders[udid]; active {
		s.mu.Unlock()
		return fmt.Errorf("already recording %s", udid)
	}
	s.mu.Unlock()

	// Snapshot outside the lock — the first tree fetch can cold-start the
	// XCUITest runner and take seconds.
	rec, err := NewRecorder(ctx, appID, func(ctx context.Context) ([]runner.Node, error) {
		return s.trees.Snapshot(ctx, udid, appID)
	})
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, active := s.recorders[udid]; active {
		return fmt.Errorf("already recording %s", udid)
	}
	s.recorders[udid] = rec
	return nil
}

// Stop ends udid's recording and returns the exported Maestro YAML, the
// replay guard sidecar, and the structured steps. The flow and the guard
// are separate artifacts on purpose — see ExportGuard.
func (s *Service) Stop(udid string) (yaml, guard string, steps []Step, err error) {
	s.mu.Lock()
	rec, ok := s.recorders[udid]
	delete(s.recorders, udid)
	s.mu.Unlock()
	if !ok {
		return "", "", nil, fmt.Errorf("not recording %s", udid)
	}
	steps = rec.Finish()
	return ExportMaestro(rec.AppID(), steps), ExportGuard(rec.AppID(), steps), steps, nil
}

// Status reports whether udid is recording and the steps so far.
func (s *Service) Status(udid string) (bool, []Step) {
	s.mu.Lock()
	rec, ok := s.recorders[udid]
	s.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, rec.Steps()
}

// OnFrame observes one raw input frame headed for udid's sidecar. A no-op
// unless the device is recording.
func (s *Service) OnFrame(udid string, frame []byte) {
	s.mu.Lock()
	rec, ok := s.recorders[udid]
	s.mu.Unlock()
	if !ok {
		return
	}
	if ev, valid := input.Decode(frame); valid {
		rec.OnEvent(ev)
	}
}
