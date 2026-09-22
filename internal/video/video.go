// Package video relays H.264 streams from devicedeck-video sidecars to
// any number of subscribers. Each subscriber gets the cached decoder
// description immediately, a fresh keyframe shortly after (requested from
// the sidecar), and drop-to-keyframe flow control: when a slow subscriber
// falls behind, delta frames are dropped and delivery resumes at the next
// keyframe, so it never renders deltas against a stale reference (tearing).
package video

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/logging"
)

// Message type bytes, as emitted by devicedeck-video.
const (
	TypeDescription byte = 1
	TypeKeyframe    byte = 2
	TypeDelta       byte = 3
	// TypeStill is a PNG snapshot of the current screen. Android capture
	// emits one on start and per keyframe request because screenrecord
	// produces H.264 only while pixels change — without a still, a viewer
	// joining a static screen would see nothing until the next change.
	TypeStill byte = 4
)

// subscriberBuffer is each subscriber's frame queue. Small keeps latency
// bounded: a viewer that can't drain 16 frames is behind by definition.
const subscriberBuffer = 16

// idleGrace is how long a session keeps capturing after its last viewer
// leaves. Long enough to absorb page reloads and websocket reconnects;
// short enough that capture never runs unwatched for hours — a 30fps
// framebuffer poller with no audience is pure load on SimRenderServer,
// which has been observed to destabilize under it (2026-08-18). Var, not
// const, so tests can shorten it.
var idleGrace = 60 * time.Second

type subscriber struct {
	ch      chan []byte
	needKey bool
}

// Session owns one devicedeck-video process and its subscribers.
type Session struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	// done closes once the sidecar process has exited and been reaped;
	// Close waits on it so shutdown is graceful-first.
	done chan struct{}

	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	description []byte
	// still caches the last TypeStill frame: Android's gRPC capture
	// sends the current screen once when its stream starts, so a viewer
	// joining a static screen later would otherwise see nothing until
	// the next pixel change.
	still     []byte
	closed    bool
	idleTimer *time.Timer
}

// StartSession launches binPath capturing udid and begins demuxing its
// stdout. The session dies (and Wait's error is discarded) when the
// process exits; subscribers see their channels closed.
//
// The process deliberately does NOT inherit the caller's context: a
// session outlives the subscriber whose request happened to start it —
// inheriting that context would kill capture for every other viewer when
// the first one disconnects. Lifetime is owned by Close/Manager.
func StartSession(_ context.Context, binPath, udid string, fps int) (*Session, error) {
	return startSessionCmd(exec.Command(binPath, udid, strconv.Itoa(fps)), udid)
}

// startSessionCmd is StartSession parametrized on the capture command, so
// platforms with different capture processes (the iOS Swift sidecar, the
// Android `devicedeck _video-android` subcommand) share one session. The
// process's stderr goes to the terminal and to video-<udid>.log.
func startSessionCmd(cmd *exec.Cmd, udid string) (*Session, error) {
	cmd.Stderr = logging.Component("video-" + udid)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("video stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("video stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start video capture %s: %w", cmd.Path, err)
	}
	s := &Session{cmd: cmd, stdin: stdin, done: make(chan struct{}), subscribers: make(map[*subscriber]struct{})}
	go func() {
		_ = ReadFrames(stdout, s.dispatch)
		_ = cmd.Wait()
		s.shutdown()
		close(s.done)
	}()
	return s, nil
}

// Subscribe registers a viewer. The returned channel yields framed
// messages ([type:u8][payload]) and closes when the session ends; cancel
// unregisters. The decoder description (if already known) is delivered
// first, and a fresh keyframe is requested from the sidecar.
func (s *Session) Subscribe() (<-chan []byte, func(), error) {
	sub := &subscriber{ch: make(chan []byte, subscriberBuffer), needKey: true}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("video session ended")
	}
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	s.subscribers[sub] = struct{}{}
	if s.description != nil {
		sub.ch <- append([]byte{TypeDescription}, s.description...)
	}
	if s.still != nil {
		sub.ch <- append([]byte{TypeStill}, s.still...)
	}
	s.mu.Unlock()

	if err := s.requestKeyframe(); err != nil {
		s.unsubscribe(sub)
		return nil, nil, err
	}
	return sub.ch, func() { s.unsubscribe(sub) }, nil
}

// Close ends the sidecar gracefully: stdin EOF tells it to detach from
// the framebuffer and exit 0. Killing it outright severs its SimulatorKit
// connection mid-frame — observed (2026-08-18) to crash SimRenderServer,
// which can cascade into CoreSimulator shutting the whole simulator down.
// Kill remains the fallback for a sidecar that fails to exit in time.
func (s *Session) Close() {
	_ = s.stdin.Close()
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.done
	}
}

// dispatch routes one demuxed sidecar frame to all subscribers.
func (s *Session) dispatch(frameType byte, payload []byte) {
	msg := append([]byte{frameType}, payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if frameType == TypeDescription {
		s.description = payload
	}
	if frameType == TypeStill {
		s.still = payload
	}
	for sub := range s.subscribers {
		s.deliver(sub, frameType, msg)
	}
}

// deliver applies drop-to-keyframe: a subscriber that missed anything
// waits for the next keyframe rather than decoding deltas against a
// stale reference. Descriptions always go through (blocking is fine —
// they arrive before any samples).
func (s *Session) deliver(sub *subscriber, frameType byte, msg []byte) {
	if frameType == TypeDelta && sub.needKey {
		return
	}
	select {
	case sub.ch <- msg:
		if frameType == TypeKeyframe {
			sub.needKey = false
		}
	default:
		sub.needKey = true
	}
}

func (s *Session) requestKeyframe() error {
	_, err := s.stdin.Write([]byte{'K'})
	return err
}

func (s *Session) unsubscribe(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subscribers[sub]; ok {
		delete(s.subscribers, sub)
		close(sub.ch)
	}
	if len(s.subscribers) == 0 && !s.closed && s.idleTimer == nil {
		s.idleTimer = time.AfterFunc(idleGrace, s.closeIfIdle)
	}
}

// closeIfIdle ends the session if the idle grace elapsed with no viewer
// returning. A viewer racing the timer may briefly see a closed channel;
// its reconnect lands in Manager.Subscribe, which replaces dead sessions.
func (s *Session) closeIfIdle() {
	s.mu.Lock()
	idle := len(s.subscribers) == 0 && !s.closed
	s.mu.Unlock()
	if idle {
		s.Close()
	}
}

func (s *Session) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for sub := range s.subscribers {
		delete(s.subscribers, sub)
		close(sub.ch)
	}
}

// ReadFrames demuxes the sidecar's [type:u8][length:u32BE][payload]
// stream, invoking handle per frame until EOF or a read error.
func ReadFrames(r io.Reader, handle func(frameType byte, payload []byte)) error {
	header := make([]byte, 5)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(header[1:])
		payload := make([]byte, length)
		if _, err := io.ReadFull(r, payload); err != nil {
			return err
		}
		handle(header[0], payload)
	}
}

// Manager lazily starts and caches one Session per simulator UDID.
type Manager struct {
	binPath  string
	fps      int
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager builds a Manager launching sidecars from binPath at fps.
func NewManager(binPath string, fps int) *Manager {
	return &Manager{binPath: binPath, fps: fps, sessions: make(map[string]*Session)}
}

// Subscribe attaches a viewer to udid's stream, starting the sidecar on
// first use. A session that ended (sim shut down) is replaced.
func (m *Manager) Subscribe(ctx context.Context, udid string) (<-chan []byte, func(), error) {
	s, err := m.session(ctx, udid)
	if err != nil {
		return nil, nil, err
	}
	ch, cancel, err := s.Subscribe()
	if err == nil {
		return ch, cancel, nil
	}
	// Session died since it was cached — replace it once.
	m.drop(udid, s)
	s, err = m.session(ctx, udid)
	if err != nil {
		return nil, nil, err
	}
	return s.Subscribe()
}

// CloseAll ends every cached session.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for udid, s := range m.sessions {
		s.Close()
		delete(m.sessions, udid)
	}
}

func (m *Manager) session(ctx context.Context, udid string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[udid]; ok {
		return s, nil
	}
	s, err := m.start(ctx, udid)
	if err != nil {
		slog.Error("video capture failed to start", "udid", udid, "err", err)
		return nil, err
	}
	slog.Info("video capture started", "udid", udid)
	m.sessions[udid] = s
	return s, nil
}

// start launches the platform-appropriate capture process.
func (m *Manager) start(_ context.Context, udid string) (*Session, error) {
	cmd, err := m.captureCommand(udid)
	if err != nil {
		return nil, err
	}
	return startSessionCmd(cmd, udid)
}

// osExecutable resolves the running binary's path. Var so tests can
// force the (otherwise unfakeable) resolution error captureCommand
// wraps when re-invoking devicedeck for Android capture.
var osExecutable = os.Executable

// captureCommand picks the capture process for a device: the Swift
// sidecar for simulators; for Android, this very binary re-invoked with
// a hidden subcommand, so the single-binary shape (brief §11.3) holds
// without a second sidecar.
func (m *Manager) captureCommand(udid string) (*exec.Cmd, error) {
	if IsAndroidSerial(udid) {
		exe, err := osExecutable()
		if err != nil {
			return nil, fmt.Errorf("resolve devicedeck binary: %w", err)
		}
		return exec.Command(exe, "_video-android", udid), nil
	}
	return exec.Command(m.binPath, udid, strconv.Itoa(m.fps)), nil
}

func (m *Manager) drop(udid string, old *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[udid] == old {
		delete(m.sessions, udid)
	}
	old.Close()
}
