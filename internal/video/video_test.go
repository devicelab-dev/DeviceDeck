package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func frame(t byte, payload []byte) []byte {
	buf := []byte{t, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(buf[1:], uint32(len(payload)))
	return append(buf, payload...)
}

func TestReadFrames(t *testing.T) {
	stream := bytes.Join([][]byte{
		frame(TypeDescription, []byte{0x01, 0x64}),
		frame(TypeKeyframe, []byte("KEY")),
		frame(TypeDelta, []byte("D")),
	}, nil)
	var got [][]byte
	err := ReadFrames(bytes.NewReader(stream), func(ft byte, payload []byte) {
		got = append(got, append([]byte{ft}, payload...))
	})
	if err != io.EOF {
		t.Fatalf("err = %v, want EOF", err)
	}
	if len(got) != 3 || got[0][0] != TypeDescription || got[1][0] != TypeKeyframe || got[2][0] != TypeDelta {
		t.Fatalf("frames = %x", got)
	}
	if string(got[1][1:]) != "KEY" {
		t.Errorf("keyframe payload = %q", got[1][1:])
	}
}

func TestReadFramesTruncated(t *testing.T) {
	full := frame(TypeKeyframe, []byte("KEYDATA"))
	if err := ReadFrames(bytes.NewReader(full[:6]), func(byte, []byte) {}); err == nil {
		t.Fatal("expected error on truncated payload")
	}
	if err := ReadFrames(bytes.NewReader(full[:3]), func(byte, []byte) {}); err == nil {
		t.Fatal("expected error on truncated header")
	}
}

// newSession builds a Session without a process for pure dispatch tests.
func newSession() *Session {
	return &Session{subscribers: make(map[*subscriber]struct{})}
}

func subscribeQuietly(s *Session) (*subscriber, <-chan []byte) {
	sub := &subscriber{ch: make(chan []byte, subscriberBuffer), needKey: true}
	s.mu.Lock()
	s.subscribers[sub] = struct{}{}
	if s.description != nil {
		sub.ch <- append([]byte{TypeDescription}, s.description...)
	}
	s.mu.Unlock()
	return sub, sub.ch
}

func drain(ch <-chan []byte) [][]byte {
	var out [][]byte
	for {
		select {
		case msg := <-ch:
			out = append(out, msg)
		default:
			return out
		}
	}
}

func TestDispatchCachesDescriptionForLateJoiners(t *testing.T) {
	s := newSession()
	s.dispatch(TypeDescription, []byte{0x01, 0x64})
	_, ch := subscribeQuietly(s)
	msgs := drain(ch)
	if len(msgs) != 1 || msgs[0][0] != TypeDescription {
		t.Fatalf("late joiner got %x, want cached description", msgs)
	}
}

func TestDropToKeyframe(t *testing.T) {
	s := newSession()
	sub, ch := subscribeQuietly(s)

	// A fresh subscriber needs a keyframe: deltas must be withheld.
	s.dispatch(TypeDelta, []byte("early-delta"))
	if msgs := drain(ch); len(msgs) != 0 {
		t.Fatalf("delta before first keyframe delivered: %x", msgs)
	}
	s.dispatch(TypeKeyframe, []byte("K1"))
	s.dispatch(TypeDelta, []byte("D1"))
	msgs := drain(ch)
	if len(msgs) != 2 || msgs[0][0] != TypeKeyframe || msgs[1][0] != TypeDelta {
		t.Fatalf("after keyframe: %x", msgs)
	}

	// Overflow the buffer: subscriber must be marked needKey and deltas
	// suppressed until the next keyframe.
	for i := 0; i < subscriberBuffer+5; i++ {
		s.dispatch(TypeDelta, []byte("flood"))
	}
	if !sub.needKey {
		t.Fatal("overflowed subscriber not marked needKey")
	}
	drain(ch)
	s.dispatch(TypeDelta, []byte("post-overflow-delta"))
	if msgs := drain(ch); len(msgs) != 0 {
		t.Fatalf("delta delivered while needKey: %x", msgs)
	}
	s.dispatch(TypeKeyframe, []byte("K2"))
	msgs = drain(ch)
	if len(msgs) != 1 || msgs[0][0] != TypeKeyframe {
		t.Fatalf("recovery keyframe: %x", msgs)
	}
}

func TestShutdownClosesSubscribers(t *testing.T) {
	s := newSession()
	_, ch := subscribeQuietly(s)
	s.shutdown()
	if _, open := <-ch; open {
		t.Fatal("channel still open after shutdown")
	}
	if _, _, err := s.Subscribe(); err == nil {
		t.Fatal("Subscribe after shutdown must fail")
	}
}

// stubVideo mimics devicedeck-video: emits a description + keyframe on
// start, then a keyframe per 'K' read from stdin.
func stubVideo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "stub-video")
	script := `#!/bin/bash
emit() { # type, payload
  printf "$(printf '\\x%02x' $1)"
  printf '\x00\x00\x00\x04'
  printf '%s' "$2"
}
emit 1 DESC
while IFS= read -r -n1 c; do
  [ "$c" = "K" ] && emit 2 KEYF
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIdleSessionReaped(t *testing.T) {
	old := idleGrace
	idleGrace = 100 * time.Millisecond
	defer func() { idleGrace = old }()

	s, err := StartSession(context.Background(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	_, cancel, err := s.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("session not reaped after idle grace")
	}
}

func TestResubscribeCancelsIdleReap(t *testing.T) {
	old := idleGrace
	idleGrace = 150 * time.Millisecond
	defer func() { idleGrace = old }()

	s, err := StartSession(context.Background(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	_, cancel, err := s.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	// A viewer returning within the grace keeps the session alive.
	_, cancel2, err := s.Subscribe()
	if err != nil {
		t.Fatalf("re-Subscribe: %v", err)
	}
	defer cancel2()
	select {
	case <-s.done:
		t.Fatal("session reaped despite an active subscriber")
	case <-time.After(400 * time.Millisecond):
	}
	s.Close()
}

func TestCloseKillsStuckSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stuck-video")
	// Detaches from stdin so EOF never reaches it: only Close's kill
	// fallback can end the process. exec (not a child) keeps the stdout
	// pipe owned by the killed pid, so reaping is immediate.
	script := "#!/bin/bash\nexec 0<&-\nexec sleep 60\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := StartSession(context.Background(), path, "booted", 30)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	start := time.Now()
	s.Close()
	if elapsed := time.Since(start); elapsed < 3*time.Second {
		t.Errorf("Close returned in %v — kill fallback should engage only after the grace window", elapsed)
	}
	select {
	case <-s.done:
	default:
		t.Error("sidecar not reaped after Close")
	}
}

func TestSessionEndToEnd(t *testing.T) {
	s, err := StartSession(context.Background(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer s.Close()

	ch, cancel, err := s.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	var got [][]byte
	deadline := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case msg, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed early; got %x", got)
			}
			got = append(got, msg)
		case <-deadline:
			t.Fatalf("timed out; got %x", got)
		}
	}
	if got[0][0] != TypeDescription || string(got[0][1:]) != "DESC" {
		t.Errorf("first message = %x", got[0])
	}
	if got[1][0] != TypeKeyframe || string(got[1][1:]) != "KEYF" {
		t.Errorf("second message = %x", got[1])
	}
}

func TestManagerReplacesDeadSession(t *testing.T) {
	m := NewManager(stubVideo(t), 30)
	defer m.CloseAll()

	_, cancel, err := m.Subscribe(context.Background(), "UDID-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	// Kill the cached session's process; the next Subscribe must replace it.
	m.mu.Lock()
	old := m.sessions["UDID-1"]
	m.mu.Unlock()
	old.Close()
	// Wait for shutdown to land.
	deadline := time.After(5 * time.Second)
	for {
		old.mu.Lock()
		closed := old.closed
		old.mu.Unlock()
		if closed {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session never marked closed")
		case <-time.After(10 * time.Millisecond):
		}
	}

	ch, cancel, err := m.Subscribe(context.Background(), "UDID-1")
	if err != nil {
		t.Fatalf("Subscribe after death: %v", err)
	}
	defer cancel()
	select {
	case msg := <-ch:
		if msg[0] != TypeDescription {
			t.Errorf("first message = %x", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message from replacement session")
	}
}

func TestManagerStartFailure(t *testing.T) {
	m := NewManager("/nonexistent/devicedeck-video", 30)
	if _, _, err := m.Subscribe(context.Background(), "UDID-1"); err == nil {
		t.Fatal("expected start error")
	}
}

// Close ends one device's capture; the next viewer starts a new one.
func TestManagerClose(t *testing.T) {
	m := NewManager(stubVideo(t), 30)
	defer m.CloseAll()
	_, cancel, err := m.Subscribe(context.Background(), "UDID-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	m.Close("UDID-1")
	m.Close("UDID-2") // never captured: a no-op
	m.mu.Lock()
	n := len(m.sessions)
	m.mu.Unlock()
	if n != 0 {
		t.Errorf("%d sessions left after Close", n)
	}
}
