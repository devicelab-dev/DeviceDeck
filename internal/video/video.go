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
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Message type bytes, as emitted by devicedeck-video.
const (
	TypeDescription byte = 1
	TypeKeyframe    byte = 2
	TypeDelta       byte = 3
)

// subscriberBuffer is each subscriber's frame queue. Small keeps latency
// bounded: a viewer that can't drain 16 frames is behind by definition.
const subscriberBuffer = 16

type subscriber struct {
	ch      chan []byte
	needKey bool
}

// Session owns one devicedeck-video process and its subscribers.
type Session struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	description []byte
	closed      bool
}

// StartSession launches binPath capturing udid and begins demuxing its
// stdout. The session dies (and Wait's error is discarded) when the
// process exits; subscribers see their channels closed.
func StartSession(ctx context.Context, binPath, udid string, fps int) (*Session, error) {
	cmd := exec.CommandContext(ctx, binPath, udid, strconv.Itoa(fps))
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("video stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("video stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start video sidecar %s: %w", binPath, err)
	}
	s := &Session{cmd: cmd, stdin: stdin, subscribers: make(map[*subscriber]struct{})}
	go func() {
		_ = ReadFrames(stdout, s.dispatch)
		_ = cmd.Wait()
		s.shutdown()
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
	s.subscribers[sub] = struct{}{}
	if s.description != nil {
		sub.ch <- append([]byte{TypeDescription}, s.description...)
	}
	s.mu.Unlock()

	if err := s.requestKeyframe(); err != nil {
		s.unsubscribe(sub)
		return nil, nil, err
	}
	return sub.ch, func() { s.unsubscribe(sub) }, nil
}

// Close ends the sidecar process; subscriber channels close as a result.
func (s *Session) Close() {
	_ = s.stdin.Close()
	_ = s.cmd.Process.Kill()
}

// dispatch routes one demuxed sidecar frame to all subscribers.
func (s *Session) dispatch(frameType byte, payload []byte) {
	msg := append([]byte{frameType}, payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if frameType == TypeDescription {
		s.description = payload
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
	s, err := StartSession(ctx, m.binPath, udid, m.fps)
	if err != nil {
		return nil, err
	}
	m.sessions[udid] = s
	return s, nil
}

func (m *Manager) drop(udid string, old *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[udid] == old {
		delete(m.sessions, udid)
	}
	old.Close()
}
