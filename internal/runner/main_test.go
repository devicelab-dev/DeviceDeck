package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain points the runner packages at a throwaway home before any test
// runs. maestro-runner resolves its home once per process (sync.Once), so
// this is the only place it can be set; the engine tests below install
// driver APKs and build caches there instead of in the user's real home.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "devicedeck-runner-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	apks := filepath.Join(home, "drivers", "android")
	if err := os.MkdirAll(apks, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{"devicelab-android-driver.apk", "devicelab-android-driver-test.apk"} {
		if err := os.WriteFile(filepath.Join(apks, name), []byte("apk"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	_ = os.Setenv("MAESTRO_RUNNER_HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// toolRule is one case of a fake command-line tool: when the joined
// arguments contain match, run body (shell) instead of the default exit 0.
type toolRule struct {
	match string
	body  string
}

// isolateTools makes a directory the only place tools are found — plus
// /bin for the shell's own utilities — so no real adb, xcrun, xcodebuild
// or emulator can run, and no real device can be reached.
func isolateTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/bin")
	return dir
}

// writeTool installs a fake tool that logs each call to dir/<name>.calls and
// answers per rules, in order; anything unmatched exits 0 silently.
func writeTool(t *testing.T, dir, name string, rules []toolRule) {
	t.Helper()
	var cases strings.Builder
	for _, r := range rules {
		fmt.Fprintf(&cases, "  *%q*) %s ;;\n", r.match, r.body)
	}
	script := fmt.Sprintf("#!/bin/sh\necho \"$*\" >> %q\ncase \"$*\" in\n%sesac\nexit 0\n",
		filepath.Join(dir, name+".calls"), cases.String())
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
