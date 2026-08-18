package video

// Error-branch and routing tests that close the gaps the primary
// lifecycle tests leave: failing writers, erroring readers, command
// construction, and the gRPC-first path through RunAndroidCapture.

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/devicelab-dev/DeviceDeck/internal/emugrpc"
)

// failWriter errors after n successful writes.
type failWriter struct{ n int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("sink full")
	}
	f.n--
	return len(p), nil
}

// errReader yields some data then a non-EOF error.
type errReader struct {
	data []byte
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func TestRepackWriterAndReaderErrors(t *testing.T) {
	sps := nal(0x67, 0x42, 0xC0, 0x32)
	pps := nal(0x68, 0xCE)
	idr := nal(0x65, frameStart, 0x11)
	stream := annexb(true, sps, pps, idr, nal(0x41, frameStart, 0x22))

	t.Run("description write fails", func(t *testing.T) {
		if err := RepackAnnexB(bytes.NewReader(stream), &failWriter{n: 0}); err == nil {
			t.Fatal("expected description write error")
		}
	})
	t.Run("sample write fails", func(t *testing.T) {
		if err := RepackAnnexB(bytes.NewReader(stream), &failWriter{n: 1}); err == nil {
			t.Fatal("expected sample write error")
		}
	})
	t.Run("reader error propagates", func(t *testing.T) {
		r := &errReader{data: annexb(true, sps, pps, idr), err: errors.New("pipe burst")}
		err := RepackAnnexB(r, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "pipe burst") {
			t.Fatalf("err = %v, want reader error", err)
		}
	})
}

func TestCaptureCommandRouting(t *testing.T) {
	m := NewManager("/opt/devicedeck-video", 30)

	ios, err := m.captureCommand("EB69B42A-4763-4A33-AF0F-CD233F721951")
	if err != nil {
		t.Fatal(err)
	}
	if ios.Path != "/opt/devicedeck-video" || ios.Args[2] != "30" {
		t.Errorf("iOS argv = %v, want sidecar with fps", ios.Args)
	}

	android, err := m.captureCommand("emulator-5554")
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	want := []string{exe, "_video-android", "emulator-5554"}
	if strings.Join(android.Args, " ") != strings.Join(want, " ") {
		t.Errorf("android argv = %v, want self-invocation %v", android.Args, want)
	}
}

func TestStartSessionCmdPipeErrors(t *testing.T) {
	t.Run("stdin already wired", func(t *testing.T) {
		cmd := exec.Command("true")
		cmd.Stdin = os.Stdin
		if _, err := startSessionCmd(cmd); err == nil {
			t.Fatal("expected stdin pipe error")
		}
	})
	t.Run("stdout already wired", func(t *testing.T) {
		cmd := exec.Command("true")
		cmd.Stdout = os.Stdout
		if _, err := startSessionCmd(cmd); err == nil {
			t.Fatal("expected stdout pipe error")
		}
	})
	t.Run("binary missing", func(t *testing.T) {
		if _, err := startSessionCmd(exec.Command("/nonexistent/devicedeck-video")); err == nil {
			t.Fatal("expected start error")
		}
	})
}

func TestSubscribeOnDeadSession(t *testing.T) {
	s, err := StartSession(t.Context(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, _, err := s.Subscribe(); err == nil {
		t.Fatal("expected error subscribing to a closed session")
	}
}

func TestRunAndroidCapturePrefersGRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeEmulator{frames: [][]byte{[]byte("grpc-frame")}, gotAuth: make(chan string, 1)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	old := discoverEndpoint
	discoverEndpoint = func(string) (emulatorEndpoint, error) {
		return emulatorEndpoint{addr: lis.Addr().String(), token: "t"}, nil
	}
	defer func() { discoverEndpoint = old }()

	stdinR, stdinW := io.Pipe()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() { done <- RunAndroidCapture("emulator-0000", stdinR, out) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !bytes.Contains(out.Bytes(), []byte("grpc-frame")) {
		time.Sleep(20 * time.Millisecond)
	}
	_ = stdinW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAndroidCapture: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not exit")
	}
	if !bytes.Contains(out.Bytes(), []byte("grpc-frame")) {
		t.Error("gRPC frame never reached the output")
	}
}

func TestRunAndroidCaptureFallsBackToScreenrecord(t *testing.T) {
	stubADB(t)
	old := discoverEndpoint
	// Discovery succeeds but the endpoint is dead — capture must fall
	// through to the screenrecord path and still stream.
	discoverEndpoint = func(string) (emulatorEndpoint, error) {
		return emulatorEndpoint{addr: "127.0.0.1:1", token: "t"}, nil
	}
	defer func() { discoverEndpoint = old }()

	stdinR, stdinW := io.Pipe()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() { done <- RunAndroidCapture("emulator-0000", stdinR, out) }()

	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) && !bytes.Contains(out.Bytes(), []byte("PNGBYTES")) {
		time.Sleep(50 * time.Millisecond)
	}
	_ = stdinW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunAndroidCapture: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not exit")
	}
	if !bytes.Contains(out.Bytes(), []byte("PNGBYTES")) {
		t.Error("screenrecord fallback never produced the still")
	}
}

func TestRequestKeyframeKillsMatureCycle(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &androidCapture{
		serial:    "emulator-0000",
		out:       &syncWriter{w: io.Discard},
		current:   cmd,
		startedAt: time.Now().Add(-time.Minute), // well past the debounce
	}
	c.requestKeyframe()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done: // killed
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("mature cycle was not killed by keyframe request")
	}
}

func TestCycleResultRepackError(t *testing.T) {
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: io.Discard}}
	repackErr := errors.New("misaligned")
	if got := c.cycleResult(repackErr, nil); got != repackErr {
		t.Errorf("cycleResult = %v, want repack error to win", got)
	}
	if got := c.cycleResult(nil, nil); got == nil {
		t.Error("clean rollover must be an error so the loop restarts")
	}
}

func TestDiscoverEmulatorRealDirs(t *testing.T) {
	// Exercises the wrapper's dir assembly (home + XDG); the serial is
	// unresolvable so any real emulator on the machine cannot match.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, err := discoverEmulator("emulator-99999"); err == nil {
		t.Fatal("expected discovery miss")
	}
}

func TestSetCancelAfterCloseFiresImmediately(t *testing.T) {
	c := &androidCapture{out: &syncWriter{w: io.Discard}, closed: true}
	fired := false
	c.setCancel(func() { fired = true })
	if !fired {
		t.Error("cancel registered after shutdown must fire immediately")
	}
}

func TestSubscribeFailsWhenKeyframeRequestFails(t *testing.T) {
	s, err := StartSession(t.Context(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Sever the command channel without ending the session: the 'K'
	// write inside Subscribe must fail and roll the subscriber back.
	_ = s.stdin.Close()
	time.Sleep(50 * time.Millisecond)
	if _, _, err := s.Subscribe(); err == nil {
		t.Fatal("expected keyframe-request failure to fail the subscribe")
	}
}

func TestRunOnceKillsWhenShutdownRacedRegistration(t *testing.T) {
	stubADB(t)
	c := &androidCapture{serial: "emulator-0000", out: &syncWriter{w: io.Discard}, closed: true}
	// closed was set before runOnce registered the process: the kill in
	// runOnce itself must reap the cycle, and the result is a clean nil.
	if err := c.runOnce(); err != nil {
		t.Fatalf("runOnce after racing shutdown: %v", err)
	}
}

func TestPumpStopsOnWriteError(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeEmulator{frames: [][]byte{[]byte("f1"), []byte("f2")}, gotAuth: make(chan string, 1)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	c := &androidCapture{out: &syncWriter{w: &failWriter{n: 0}}}
	client := emugrpc.NewEmulatorControllerClient(mustDial(t, lis.Addr().String()))
	if err := c.pumpScreenshotStream(client); err == nil || !strings.Contains(err.Error(), "sink full") {
		t.Fatalf("err = %v, want write failure to stop the pump", err)
	}
}

func mustDial(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestSubscribeDeliversCachedDescription(t *testing.T) {
	s, err := StartSession(t.Context(), stubVideo(t), "booted", 30)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Wait for the stub's DESC frame to be cached, then join late.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		cached := s.description != nil
		s.mu.Unlock()
		if cached {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ch, cancel, err := s.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	select {
	case msg := <-ch:
		if msg[0] != TypeDescription {
			t.Errorf("first message type = %d, want cached description first", msg[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cached description not delivered to late joiner")
	}
}

func TestManagerReplacementStartFailure(t *testing.T) {
	m := NewManager(stubVideo(t), 30)
	ch, cancel, err := m.Subscribe(t.Context(), "booted")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = ch
	// Kill the cached session, then make replacement impossible: the
	// dead-session replacement path must surface the start error.
	m.mu.Lock()
	s := m.sessions["booted"]
	m.mu.Unlock()
	s.Close()
	m.binPath = "/nonexistent/devicedeck-video"
	if _, _, err := m.Subscribe(t.Context(), "booted"); err == nil {
		t.Fatal("expected replacement start failure")
	}
}

func TestParseDiscoveryFileMissing(t *testing.T) {
	if kv := parseDiscoveryFile("/nonexistent/pid_1.ini"); len(kv) != 0 {
		t.Errorf("kv = %v, want empty for unreadable file", kv)
	}
}

func TestStreamViaGRPCBadTarget(t *testing.T) {
	c := &androidCapture{out: &syncWriter{w: io.Discard}}
	if err := c.streamViaGRPC(emulatorEndpoint{addr: "bad\x00target", token: "t"}); err == nil {
		t.Fatal("expected dial construction error")
	}
}

func TestPumpSkipsEmptyFrames(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeEmulator{frames: [][]byte{{}, []byte("real")}, gotAuth: make(chan string, 1)}
	srv := grpc.NewServer()
	emugrpc.RegisterEmulatorControllerServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	out := &syncBuffer{}
	c := &androidCapture{out: &syncWriter{w: out}}
	done := make(chan error, 1)
	go func() {
		done <- c.streamViaGRPC(emulatorEndpoint{addr: lis.Addr().String(), token: "t"})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !bytes.Contains(out.Bytes(), []byte("real")) {
		time.Sleep(20 * time.Millisecond)
	}
	c.mu.Lock()
	c.closed = true
	if c.cancelStream != nil {
		c.cancelStream()
	}
	c.mu.Unlock()
	<-done

	stills := 0
	_ = ReadFrames(bytes.NewReader(out.Bytes()), func(ft byte, p []byte) {
		if ft == TypeStill {
			stills++
		}
	})
	if stills != 1 {
		t.Errorf("stills = %d, want empty frame skipped and real frame kept", stills)
	}
}
