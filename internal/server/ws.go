package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/devicelab-dev/DeviceDeck/internal/input"
)

// TakenOverPrefix starts the close reason a driver gets when another
// client takes its device over, so a page can say what happened to it
// rather than report a refusal it did nothing to cause.
const TakenOverPrefix = "taken over by "

// wsCloseReasonMax is the WebSocket close-reason limit: the control
// frame carries at most 125 bytes, of which 2 are the status code.
const wsCloseReasonMax = 123

// truncateReason fits a message into a close frame. Writing an oversized
// reason fails the close outright, which would replace an explanatory
// refusal with a silent drop — the exact outcome this is here to avoid.
func truncateReason(reason string) string {
	if len(reason) <= wsCloseReasonMax {
		return reason
	}
	const ellipsis = "…" // three bytes, not one
	cut := wsCloseReasonMax - len(ellipsis)
	// Never split a multi-byte rune: an invalid tail would make the
	// frame unreadable rather than merely shortened.
	for cut > 0 && !utf8.RuneStart(reason[cut]) {
		cut--
	}
	return reason[:cut] + ellipsis
}

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
	defer func() { _ = conn.Close(websocket.StatusInternalError, "stream ended") }()

	ctx := r.Context()
	for {
		select {
		case msg, ok := <-frames:
			if !ok {
				_ = conn.Close(websocket.StatusNormalClosure, "capture ended")
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

// Input connections are pinged on this cadence. A driver that dies
// without closing its socket — a killed browser, a slept laptop, a CI
// job that was cancelled — leaves the connection open from this side, so
// conn.Read blocks for ever and the device's claim is never released.
// Every later client is then refused on behalf of a driver that no
// longer exists, and on the device page the refusal is nearly invisible.
// A ping is the only thing that distinguishes a quiet driver from a dead
// one. The interval is a compromise: long enough to be nothing on the
// wire, short enough that a device is reclaimable within half a minute.
const (
	inputPingInterval = 15 * time.Second
	inputPingTimeout  = 10 * time.Second
)

// watchLiveness pings conn until a ping goes unanswered, then closes it
// so the handler's blocked Read returns and its deferred release runs.
// Closing is what frees the device; this only decides when.
// The cadence is a parameter so tests need not wait real seconds for a
// verdict; production callers pass the constants above.
func watchLiveness(ctx context.Context, conn *websocket.Conn, interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				// CloseNow, not Close: a graceful close waits for the
				// peer to acknowledge, and this peer has already been
				// measured as unable to answer — waiting was observed to
				// cost ~3s, all of it with the device still claimed.
				_ = conn.CloseNow()
				return
			}
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
	defer func() { _ = conn.Close(websocket.StatusInternalError, "input ended") }()

	ctx := r.Context()
	udid := r.PathValue("udid")

	// One driver per device — see inputOwners. The refusal carries who
	// holds it, because the failure it prevents is otherwise silent.
	// ?takeover=1 disconnects the holder instead (the console's Take over).
	release, err := s.inputs.acquire(udid, r.RemoteAddr, kicker(conn), r.URL.Query().Get("takeover") == "1")
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, truncateReason(err.Error()))
		return
	}
	defer release()

	// Only after the claim: a refused connection has nothing to release,
	// and pinging it would keep a doomed socket alive for no reason.
	liveCtx, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	go watchLiveness(liveCtx, conn, inputPingInterval, inputPingTimeout)

	var held heldTouch
	// Closure, not a direct defer: deferred arguments evaluate at defer
	// time, when nothing is held yet.
	defer func() { s.releaseTouch(udid, held.frames()) }()
	for {
		kind, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if err := s.inputMessage(ctx, udid, kind, raw, &held); err != nil {
			_ = conn.Close(websocket.StatusInternalError, "sidecar unavailable")
			return
		}
	}
}

// inputMessage handles one message from the input socket: a valid binary
// frame goes to the device; anything else is ignored. The device page acts
// through POST /act instead; this socket carries the console's live input.
func (s *Server) inputMessage(ctx context.Context, udid string, kind websocket.MessageType, raw []byte, held *heldTouch) error {
	if kind != websocket.MessageBinary || !input.ValidFrame(raw) {
		return nil
	}
	held.observe(raw)
	return s.sendFrame(ctx, udid, raw)
}

// inputFlusher is a FrameSender that can hold input back — the Android
// router batches typed characters into one driver call — and can be told
// to send it now.
type inputFlusher interface {
	Flush(ctx context.Context, udid string) error
}

// flushInput pushes any input the frame sender is holding for udid to the
// device. A failure is logged, not fatal: the mark is recorded anyway, and
// the settle that follows reports what the device actually shows.
func (s *Server) flushInput(ctx context.Context, udid string) {
	f, ok := s.frames.(inputFlusher)
	if !ok {
		return
	}
	if err := f.Flush(ctx, udid); err != nil {
		slog.Warn("input flush failed", "udid", udid, "err", err)
	}
}

// kicker disconnects conn, telling it why: another client took its device
// over, or its session ended. Closing waits for the peer's
// acknowledgement, which whoever kicked it must not wait for too.
func kicker(conn *websocket.Conn) func(why string) {
	return func(why string) {
		go func() { _ = conn.Close(websocket.StatusPolicyViolation, truncateReason(why)) }()
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
