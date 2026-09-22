package video

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// orphanEnv marks a re-executed test binary that should play the orphaned
// capture process: run watchOrphaned and nothing else.
const orphanEnv = "DEVICEDECK_TEST_ORPHAN_PIDFILE"

// TestMain runs the orphan role when asked. It must be here, not in an
// init: coverage from a child is only recorded once the test main has
// started.
func TestMain(m *testing.M) {
	if pidFile := os.Getenv(orphanEnv); pidFile != "" {
		_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
		watchOrphaned() // exits once the parent is gone
		os.Exit(1)      // reached only if watchOrphaned returns
	}
	os.Exit(m.Run())
}

// TestWatchOrphanedExitsWhenTheServerDies starts the watcher in a process
// whose parent exits at once, so it is reparented to launchd — the state a
// capture process is in when the server is killed — and checks it exits
// on its own. Coverage from that process is merged by `go test -cover`.
func TestWatchOrphanedExitsWhenTheServerDies(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	args := strings.Join(quoteAll(append([]string{os.Args[0]}, os.Args[1:]...)), " ")
	sh := exec.Command("/bin/sh", "-c", args+" >/dev/null 2>&1 &")
	sh.Env = append(os.Environ(), orphanEnv+"="+pidFile)
	if err := sh.Run(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	pid := 0
	for time.Now().Before(deadline) {
		if pid == 0 {
			data, _ := os.ReadFile(pidFile)
			pid, _ = strconv.Atoi(string(data))
		} else if syscall.Kill(pid, 0) != nil {
			return // the orphan noticed and exited
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pid != 0 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	t.Fatalf("orphaned watcher (pid %d) did not exit", pid)
}

// quoteAll single-quotes each argument for /bin/sh.
func quoteAll(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return out
}
