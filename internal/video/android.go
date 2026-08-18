package video

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/platform"
)

// IsAndroidSerial reports whether udid names an adb device rather than
// an iOS simulator. Thin delegate to the shared platform check.
func IsAndroidSerial(udid string) bool {
	return platform.IsAndroidSerial(udid)
}

// screenrecordLimit is screenrecord's own per-invocation cap (3 minutes);
// asking for just under it lets the process end cleanly on its schedule
// rather than being cut off mid-write.
const screenrecordLimit = "179"

// keyframeDebounce ignores keyframe requests arriving within this window
// of the last capture (re)start: every restart opens with SPS/PPS + IDR,
// so a burst of joining viewers needs only one restart.
const keyframeDebounce = 2 * time.Second

// androidCapture owns one screenrecord process at a time and restarts it
// on exit (the 3-minute cap) or on keyframe request.
type androidCapture struct {
	serial string
	out    io.Writer

	mu           sync.Mutex
	current      *exec.Cmd
	startedAt    time.Time
	closed       bool // stdin saw EOF: shut down instead of restarting
	cancelStream func()
}

// RunAndroidCapture streams the emulator's screen as sidecar protocol
// frames on out — the Android counterpart of the devicedeck-video Swift
// sidecar, run as a hidden subcommand of devicedeck itself so no extra
// binary ships. 'K' on in requests a keyframe; EOF on in ends the
// session.
func RunAndroidCapture(serial string, in io.Reader, out io.Writer) error {
	c := &androidCapture{serial: serial, out: &syncWriter{w: out}}
	go c.readCommands(in)
	go watchOrphaned()

	// Preferred path: the emulator's host-side gRPC screenshot stream —
	// no adb, no 3-minute cap, current frame on subscribe. screenrecord
	// below is the fallback for devices without a discovery file.
	if ep, err := discoverEndpoint(serial); err == nil {
		gerr := c.streamViaGRPC(ep)
		if gerr == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "devicedeck: emulator grpc capture failed (%v); falling back to screenrecord\n", gerr)
	}
	return c.runScreenrecordLoop()
}

// runScreenrecordLoop drives screenrecord cycles until shutdown. The
// device's own encoder produces H.264; we only repackage. A keyframe
// request restarts the cycle (each restart begins with SPS/PPS + IDR).
func (c *androidCapture) runScreenrecordLoop() error {
	// screenrecord emits H.264 only while pixels change, so a session
	// opened on a static screen would otherwise stay black — seed the
	// viewer with a snapshot of the current content.
	c.emitStill()

	consecutiveFast := 0
	for {
		started := time.Now()
		err := c.runOnce()
		if err == nil {
			return nil // stdin closed → deliberate shutdown
		}
		// A healthy cycle runs ~3 minutes; rapid failures mean adb or the
		// emulator is gone — back off, then give up.
		if time.Since(started) < 2*time.Second {
			consecutiveFast++
			if consecutiveFast >= 5 {
				return fmt.Errorf("android capture failing repeatedly: %w", err)
			}
			time.Sleep(time.Second)
		} else {
			consecutiveFast = 0
		}
	}
}

// runOnce runs a single screenrecord cycle. Returns nil only when the
// session should end (stdin closed kills the process on purpose).
func (c *androidCapture) runOnce() error {
	cmd := exec.Command("adb", "-s", c.serial, "exec-out",
		"screenrecord", "--output-format=h264", "--time-limit="+screenrecordLimit, "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	c.mu.Lock()
	c.current = cmd
	c.startedAt = time.Now()
	closedNow := c.closed
	c.mu.Unlock()
	// Shutdown may have raced the registration above — its kill would
	// have found no process, so finish the job here or block forever.
	if closedNow {
		_ = cmd.Process.Kill()
	}

	repackErr := RepackAnnexB(stdout, c.out)
	return c.cycleResult(repackErr, cmd.Wait())
}

// cycleResult classifies a finished screenrecord cycle: nil only for a
// deliberate shutdown; everything else (including the normal 3-minute
// rollover) is an error so the outer loop restarts capture.
func (c *androidCapture) cycleResult(repackErr, waitErr error) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil
	}
	if repackErr != nil {
		return repackErr
	}
	if waitErr != nil {
		return waitErr
	}
	return fmt.Errorf("screenrecord cycle ended") // normal 3-minute rollover
}

// readCommands consumes single-byte commands: 'K' restarts capture for a
// fresh keyframe; EOF marks shutdown and kills the current process.
func (c *androidCapture) readCommands(in io.Reader) {
	buf := make([]byte, 1)
	for {
		_, err := in.Read(buf)
		if err != nil {
			c.mu.Lock()
			c.closed = true
			cur := c.current
			cancel := c.cancelStream
			c.mu.Unlock()
			if cur != nil && cur.Process != nil {
				_ = cur.Process.Kill()
			}
			if cancel != nil {
				cancel()
			}
			return
		}
		if buf[0] == 'K' {
			c.requestKeyframe()
		}
	}
}

func (c *androidCapture) requestKeyframe() {
	// The still always goes out: joining a static screen must show the
	// current content even when the video restart below is debounced.
	c.emitStill()
	c.mu.Lock()
	cur := c.current
	recent := time.Since(c.startedAt) < keyframeDebounce
	c.mu.Unlock()
	if cur == nil || cur.Process == nil || recent {
		return
	}
	_ = cur.Process.Kill()
}

// emitStill snapshots the current screen as PNG and frames it as a
// TypeStill message. Best-effort: a failed screencap only means the
// viewer waits for the next real frame.
func (c *androidCapture) emitStill() {
	png, err := exec.Command("adb", "-s", c.serial, "exec-out", "screencap", "-p").Output()
	if err != nil || len(png) == 0 {
		return
	}
	_ = WriteFrame(c.out, TypeStill, png)
}

// syncWriter serializes whole-frame writes from concurrent producers
// (the repack loop and still snapshots). WriteFrame issues one Write per
// frame, so per-Write locking is frame-atomic.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// watchOrphaned exits when the parent server dies without the pipes
// unwinding — same defense as the Swift sidecars' OrphanWatch.
//
// Coverage waiver: the os.Exit branch cannot run inside the test
// process; the loop is exercised by every capture lifecycle test.
func watchOrphaned() {
	for {
		time.Sleep(2 * time.Second)
		if os.Getppid() == 1 {
			os.Exit(0)
		}
	}
}
