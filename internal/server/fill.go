package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
)

// fillMessage is a page's fill(): set this field to this value (see
// handleAct).
type fillMessage struct {
	// App is the app under test; the iOS runner acts within it.
	App string `json:"app"`
	// ID is the field's identifier, sent only when unique on screen.
	ID string `json:"id"`
	// X, Y locate the field normalized to the screen, for recording.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// PX, PY locate it in the tree's own units, for the driver.
	PX   float64 `json:"px"`
	PY   float64 `json:"py"`
	Text string  `json:"text"`
	// Prev is how many characters the device last reported in the field.
	Prev int `json:"prev"`
}

// fieldFiller sets a field's value through the device's driver.
type fieldFiller interface {
	Fill(ctx context.Context, udid string, req runner.FillRequest) error
}

// fill records the fill for Flow Capture, then has the driver set the
// field and confirm it.
func (s *Server) fill(ctx context.Context, udid string, m fillMessage) error {
	s.capture.OnFill(udid, m.X, m.Y, m.Text)
	f, ok := s.trees.(fieldFiller)
	if !ok {
		return errors.New("this device cannot fill fields")
	}
	start := time.Now()
	err := f.Fill(ctx, udid, runner.FillRequest{
		App: m.App, Identifier: m.ID, X: m.PX, Y: m.PY, Text: m.Text, PrevLen: m.Prev,
	})
	if err != nil {
		slog.Warn("fill failed", "udid", udid, "field", m.ID, "took", time.Since(start), "err", err)
		return err
	}
	slog.Debug("fill", "udid", udid, "field", m.ID, "took", time.Since(start))
	return nil
}
