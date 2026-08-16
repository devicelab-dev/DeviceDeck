package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// readyTimeout caps the wait for the sidecar's "ready" handshake. Attaching
// to SimulatorKit is normally sub-second; a sidecar silent for this long is
// wedged. Variable so tests can shorten it.
var readyTimeout = 15 * time.Second

// Session is one running devicedeck-hid process bound to a simulator.
// Frames are written to its stdin; the write lock keeps concurrent HTTP
// handlers from interleaving partial frames.
type Session struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	mu    sync.Mutex
}

// StartSidecar launches binPath attached to udid and blocks until the
// sidecar reports "ready" on stdout (or the timeout expires). Sidecar
// stderr is passed through to our stderr for diagnostics.
//
// The process deliberately does NOT inherit the caller's context: sessions
// outlive the (often short-lived HTTP request) context that first touched
// them — tying the process to it kills the sidecar the moment that request
// completes, mid-injection. Lifetime is owned by Close/Manager.
func StartSidecar(_ context.Context, binPath, udid string) (*Session, error) {
	cmd := exec.Command(binPath, udid)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sidecar stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sidecar %s: %w", binPath, err)
	}
	if err := awaitReady(stdout); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("sidecar for %s: %w", udid, err)
	}
	return &Session{cmd: cmd, stdin: stdin}, nil
}

// Send writes one encoded frame. A write error means the sidecar died —
// the caller should drop the session and start a new one.
func (s *Session) Send(frame []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.stdin.Write(frame)
	return err
}

// Close ends the session by closing stdin (the sidecar exits on EOF) and
// reaping the process.
func (s *Session) Close() error {
	_ = s.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		_ = s.cmd.Process.Kill()
		return <-done
	}
}

func awaitReady(stdout io.Reader) error {
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	select {
	case line, ok := <-lines:
		if !ok || !strings.HasPrefix(line, "ready") {
			return fmt.Errorf("handshake failed (got %q)", line)
		}
		return nil
	case <-time.After(readyTimeout):
		return fmt.Errorf("timed out waiting for ready handshake")
	}
}

// Manager lazily starts and caches one Session per simulator UDID.
type Manager struct {
	binPath  string
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager builds a Manager that launches sidecars from binPath.
func NewManager(binPath string) *Manager {
	return &Manager{binPath: binPath, sessions: make(map[string]*Session)}
}

// Session returns the cached session for udid, starting one if needed.
func (m *Manager) Session(ctx context.Context, udid string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[udid]; ok {
		return s, nil
	}
	s, err := StartSidecar(ctx, m.binPath, udid)
	if err != nil {
		return nil, err
	}
	m.sessions[udid] = s
	return s, nil
}

// SendFrame delivers one frame to udid's sidecar, starting it on first
// use. A dead sidecar is dropped and restarted once before giving up —
// sidecars die legitimately when their simulator shuts down.
func (m *Manager) SendFrame(ctx context.Context, udid string, frame []byte) error {
	s, err := m.Session(ctx, udid)
	if err != nil {
		return err
	}
	if err := s.Send(frame); err == nil {
		return nil
	}
	m.Drop(udid)
	s, err = m.Session(ctx, udid)
	if err != nil {
		return err
	}
	return s.Send(frame)
}

// Drop removes and closes the session for udid (after a send failure).
func (m *Manager) Drop(udid string) {
	m.mu.Lock()
	s, ok := m.sessions[udid]
	delete(m.sessions, udid)
	m.mu.Unlock()
	if ok {
		_ = s.Close()
	}
}

// CloseAll shuts down every cached session.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for udid, s := range m.sessions {
		_ = s.Close()
		delete(m.sessions, udid)
	}
}
