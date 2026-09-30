package home

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var shipped = []string{"devicelab-android-driver-test.apk", "devicelab-android-driver.apk"}

func TestDir(t *testing.T) {
	t.Run("override", func(t *testing.T) {
		t.Setenv(EnvHome, "/opt/dd")
		if got, err := Dir(); err != nil || got != "/opt/dd" {
			t.Fatalf("Dir() = %q, %v", got, err)
		}
	})
	t.Run("default under the user's home", func(t *testing.T) {
		t.Setenv(EnvHome, "")
		t.Setenv("HOME", "/Users/someone")
		if got, err := Dir(); err != nil || got != "/Users/someone/.devicedeck" {
			t.Fatalf("Dir() = %q, %v", got, err)
		}
	})
	t.Run("no home directory names the override", func(t *testing.T) {
		t.Setenv(EnvHome, "")
		t.Setenv("HOME", "")
		if _, err := Dir(); err == nil || !strings.Contains(err.Error(), EnvHome) {
			t.Fatalf("err = %v, want one naming %s", err, EnvHome)
		}
	})
}

func driverDir(dir string) string { return filepath.Join(dir, "drivers", "android") }

func TestPrepareInstallsDriversAndPointsTheRunner(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(driverDir(dir), "devicelab-android-driver-test-old.apk")
	keep := filepath.Join(driverDir(dir), "notes.txt")
	changed := filepath.Join(driverDir(dir), "devicelab-android-driver.apk")
	for path, body := range map[string]string{stale: "old", keep: "mine", changed: "outdated"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(runnerHomeEnv, "/somewhere/else")
	for range 2 { // the second pass must be a no-op, not an error
		if err := Prepare(dir); err != nil {
			t.Fatal(err)
		}
	}
	if got := os.Getenv(runnerHomeEnv); got != dir {
		t.Errorf("%s = %q, want %q", runnerHomeEnv, got, dir)
	}
	for _, name := range shipped {
		want, _ := androidDrivers.ReadFile("android/" + name)
		got, err := os.ReadFile(filepath.Join(driverDir(dir), name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s not installed byte-for-byte (err %v)", name, err)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale APK survived: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("non-APK file was removed: %v", err)
	}
}

// agentFile is where Prepare installs an embedded agent file.
func agentFile(dir, embedded string) string {
	return filepath.Join(dir, "drivers", filepath.FromSlash(embedded))
}

// agentFiles lists the embedded agent's files by embedded path.
func agentFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := fs.WalkDir(iosAgent, agentRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("embedded agent: %d files, %v", len(files), err)
	}
	return files
}

func TestPrepareInstallsIOSAgent(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(dir, "drivers", filepath.FromSlash(agentRoot))
	stale := filepath.Join(agent, "simulator", "Old.app", "Old")
	outside := filepath.Join(dir, "drivers", "ios", "notes.txt")
	for _, path := range []string{stale, outside} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 { // the second pass must be a no-op, not an error
		if err := Prepare(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range agentFiles(t) {
		want, _ := iosAgent.ReadFile(p)
		got, err := os.ReadFile(agentFile(dir, p))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s not installed byte-for-byte (err %v)", p, err)
		}
	}
	runner := agentFile(dir, agentRoot+"/simulator/DevicelabIOSAgentUITests-Runner.app/DevicelabIOSAgentUITests-Runner")
	if info, err := os.Stat(runner); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("agent runner binary is not executable: %v, %v", info, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale agent file survived: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a file outside the agent folder was removed: %v", err)
	}
}

func TestFileMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want os.FileMode
	}{
		{"thin 64-bit Mach-O", []byte{0xcf, 0xfa, 0xed, 0xfe, 0x0c}, 0o700},
		{"universal Mach-O", []byte{0xca, 0xfe, 0xba, 0xbe, 0x00}, 0o700},
		{"property list", []byte("<?xml"), 0o600},
		{"empty", nil, 0o600},
	} {
		if got := fileMode(tc.data); got != tc.want {
			t.Errorf("%s: mode %o, want %o", tc.name, got, tc.want)
		}
	}
}

func TestPruneAgentCannotRemove(t *testing.T) {
	root := t.TempDir()
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "old"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	if err := pruneAgent(root, nil); err == nil || !strings.Contains(err.Error(), "remove stale agent file") {
		t.Fatalf("err = %v", err)
	}
	if err := pruneAgent(filepath.Join(root, "missing"), nil); err == nil || !strings.Contains(err.Error(), "open agent folder") {
		t.Fatalf("missing folder: %v", err)
	}
}

// blockWith puts a non-empty directory at path, which a file write, rename or
// remove cannot replace.
func blockWith(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, "x"), 0o750); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  string
	}{
		{"driver folder cannot be created", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "drivers"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, "create driver folder"},
		{"temporary file cannot be written", func(t *testing.T, dir string) {
			blockWith(t, filepath.Join(driverDir(dir), shipped[0]+".tmp"))
		}, "write driver"},
		{"driver cannot replace what is there", func(t *testing.T, dir string) {
			blockWith(t, filepath.Join(driverDir(dir), shipped[0]))
		}, "install driver"},
		{"stale driver cannot be removed", func(t *testing.T, dir string) {
			blockWith(t, filepath.Join(driverDir(dir), "old.apk"))
		}, "remove stale driver"},
		{"agent folder cannot be created", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, "drivers"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "drivers", "ios"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, "create agent folder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			if err := Prepare(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRemoveStaleUnreadableFolder(t *testing.T) {
	if err := removeStale(filepath.Join(t.TempDir(), "missing"), nil); err == nil ||
		!strings.Contains(err.Error(), "read driver folder") {
		t.Fatalf("err = %v", err)
	}
}

// TestEmbeddedDriversMatchPinnedRunner fails when a maestro-runner bump
// changes the driver APKs but `make drivers` was not run, so a release can
// never pair new runner code with an old on-device driver.
func TestEmbeddedDriversMatchPinnedRunner(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/devicelab-dev/maestro-runner").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skipf("maestro-runner module not available: %v", err)
	}
	src := filepath.Join(strings.TrimSpace(string(out)), "drivers", "android")
	for _, name := range shipped {
		want, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("pinned runner no longer ships %s: %v", name, err)
		}
		got, _ := androidDrivers.ReadFile("android/" + name)
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the pinned maestro-runner; run `make drivers`", name)
		}
	}
}

// TestEmbeddedAgentMatchesPinnedRunner is the iOS agent's drift check: the
// embedded copy must hold exactly the pinned runner's files, byte for byte.
func TestEmbeddedAgentMatchesPinnedRunner(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/devicelab-dev/maestro-runner").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		t.Skipf("maestro-runner module not available: %v", err)
	}
	src := filepath.Join(strings.TrimSpace(string(out)), "drivers")
	pinned := 0
	err = filepath.WalkDir(filepath.Join(src, filepath.FromSlash(agentRoot), "simulator"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		pinned++
		rel, _ := filepath.Rel(src, p)
		want, _ := os.ReadFile(p)
		if got, _ := iosAgent.ReadFile(filepath.ToSlash(rel)); !bytes.Equal(got, want) {
			t.Errorf("%s differs from the pinned maestro-runner; run `make drivers`", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pinned runner ships no iOS agent: %v", err)
	}
	if embedded := len(agentFiles(t)); embedded != pinned {
		t.Errorf("embedded agent has %d files, pinned runner %d; run `make drivers`", embedded, pinned)
	}
}

// TestEmbeddedDriversCarryNoLocalPaths guards the release: the redaction
// step blanks every /Users/ path in the shipped binary, and the APKs are
// embedded in it raw, so a path inside one would be blanked too and break
// the APK's signature on the device.
func TestEmbeddedDriversCarryNoLocalPaths(t *testing.T) {
	for _, name := range shipped {
		data, _ := androidDrivers.ReadFile("android/" + name)
		if bytes.Contains(data, []byte("/Users/")) {
			t.Errorf("%s contains a /Users/ path; release redaction would corrupt it", name)
		}
	}
	for _, p := range agentFiles(t) {
		data, _ := iosAgent.ReadFile(p)
		if bytes.Contains(data, []byte("/Users/")) {
			t.Errorf("%s contains a /Users/ path; release redaction would corrupt it", p)
		}
	}
}
