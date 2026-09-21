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
	yaml = ExportMaestroWithLint(rec.AppID(), steps, rec.Lint())
	if err := exportValidator(yaml); err != nil {
		return "", "", steps, err
	}
	return yaml, ExportGuard(rec.AppID(), steps), steps, nil
}

// exportValidator is validateExport, indirected through a variable so a
// test can force the rejection path Stop must handle. A correct emitter
// never produces a flow that fails validateExport, so this guard is
// otherwise unreachable through the public API — the indirection exists to
// prove Stop refuses a bad flow rather than hand one back.
var exportValidator = validateExport

// validateExport refuses a captured flow that the real runner cannot parse
// or that leans on a selector field one target platform's driver ignores.
// A capture has to replay unchanged on both iOS and Android simulators and
// on devicelab.dev real devices, so it is checked against both platforms
// before it is ever handed back — a broken flow is a failed capture, not a
// file the user discovers is wrong at replay time.
func validateExport(yaml string) error {
	// UnsupportedFields parses with the real runner and reports fields the
	// platform's driver ignores, so one call per platform covers both the
	// parseability contract (a parse error surfaces here) and the
	// portability contract — no need to parse twice.
	for _, platform := range []string{"ios", "android"} {
		bad, err := runner.UnsupportedFields([]byte(yaml), platform)
		if err != nil {
			return err
		}
		if len(bad) != 0 {
			return fmt.Errorf("captured flow uses %s-unsupported fields: %v", platform, bad)
		}
	}
	return nil
}

// Assert records a visibility assertion at a normalized point on udid's
// recording. Fails when the device is not recording, or when the point
// resolves to nothing durable enough to assert on.
func (s *Service) Assert(udid string, x, y float64) error {
	s.mu.Lock()
	rec, ok := s.recorders[udid]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("not recording %s", udid)
	}
	if !rec.Assert(x, y) {
		return fmt.Errorf("nothing to assert on at %.3f,%.3f: no element with an identifier or text", x, y)
	}
	return nil
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
