package server

import (
	"context"
	"net/http"

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
	for {
		kind, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageBinary || !input.ValidFrame(raw) {
			continue
		}
		if err := s.frames.SendFrame(ctx, udid, raw); err != nil {
			conn.Close(websocket.StatusInternalError, "sidecar unavailable")
			return
		}
	}
}
