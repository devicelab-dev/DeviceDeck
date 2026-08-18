package server

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
)

// VideoSource attaches viewers to a device's H.264 stream.
type VideoSource interface {
	Subscribe(ctx context.Context, udid string) (<-chan []byte, func(), error)
}

// handleVideoWS streams framed video messages ([type:u8][payload]) to the
// browser as binary WebSocket messages until the client leaves or the
// stream ends.
func (s *Server) handleVideoWS(w http.ResponseWriter, r *http.Request) {
	frames, cancel, err := s.video.Subscribe(r.Context(), r.PathValue("udid"))
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	defer cancel()

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusInternalError, "stream ended")

	ctx := r.Context()
	for {
		select {
		case msg, ok := <-frames:
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "capture ended")
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// handleInputWS receives raw sidecar protocol frames from the browser —
// the real-time path for streamed touches during manual driving. Each
// binary message must be exactly one valid frame; malformed frames are
// dropped (never forwarded, they would desync the sidecar's stdin).
func (s *Server) handleInputWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusInternalError, "input ended")

	ctx := r.Context()
	udid := r.PathValue("udid")
	var held heldTouch
	// Closure, not a direct defer: deferred arguments evaluate at defer
	// time, when nothing is held yet.
	defer func() { s.releaseTouch(udid, held.frames()) }()
	for {
		kind, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary || !input.ValidFrame(raw) {
			continue
		}
		held.observe(raw)
		if err := s.sendFrame(ctx, udid, raw); err != nil {
			conn.Close(websocket.StatusInternalError, "sidecar unavailable")
			return
		}
	}
}

// heldTouch tracks contacts this connection has pressed and not yet
// released, so they can be released if the connection disappears.
type heldTouch struct {
	one    bool
	two    bool
	x, y   float64
	x2, y2 float64
}

// observe folds one outgoing frame into the held-contact state.
func (h *heldTouch) observe(frame []byte) {
	ev, ok := input.Decode(frame)
	if !ok {
		return
	}
	switch ev.Kind {
	case input.EventTouch:
		h.one = ev.Phase != input.TouchUp
		h.x, h.y = ev.X, ev.Y
	case input.EventTwoFinger:
		h.two = ev.Phase != input.TouchUp
		h.x, h.y, h.x2, h.y2 = ev.X, ev.Y, ev.X2, ev.Y2
	}
}

// frames returns the releases needed to leave no contact pressed.
func (h *heldTouch) frames() [][]byte {
	var out [][]byte
	if h.one {
		out = append(out, input.Touch(input.TouchUp, h.x, h.y, input.EdgeNone))
	}
	if h.two {
		out = append(out, input.TwoFinger(input.TouchUp, h.x, h.y, h.x2, h.y2))
	}
	return out
}

// releaseTouch lifts contacts left pressed by a connection that vanished
// mid-gesture — a closed tab, a dropped network. Without it the sidecar
// has a touch-down with no matching up and the device keeps holding that
// finger, which wedges every later interaction until something else
// releases it. The request's own context is already cancelled by the
// time this runs, so the release needs a fresh one.
func (s *Server) releaseTouch(udid string, frames [][]byte) {
	if len(frames) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()
	for _, f := range frames {
		if err := s.sendFrame(ctx, udid, f); err != nil {
			return
		}
	}
}

// releaseTimeout bounds the cleanup write: the sidecar may itself be
// gone, and shutdown must not block on it.
const releaseTimeout = 2 * time.Second
