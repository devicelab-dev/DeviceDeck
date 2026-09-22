package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeMachine answers commands from a table keyed by the full command line;
// anything not in it fails as if the tool were not installed.
type fakeMachine struct {
	out    map[string]string
	exists map[string]bool
	onPath map[string]string
}

func (f fakeMachine) env() Env {
	return Env{
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if out, ok := f.out[strings.Join(append([]string{name}, args...), " ")]; ok {
				return []byte(out), nil
			}
			return nil, errors.New("not found")
		},
		LookPath: func(file string) (string, error) {
			if p, ok := f.onPath[file]; ok {
				return p, nil
			}
			return "", errors.New("not on PATH")
		},
		Exists: func(path string) bool { return f.exists[path] },
		Home:   "/Users/dev",
	}
}

const runtimesJSON = `{"runtimes":[{"name":"iOS 18.6","isAvailable":true},` +
	`{"name":"iOS 26.2","isAvailable":true},{"name":"iOS 27.0","isAvailable":false}]}`

// healthy is a Mac with everything installed, SimulatorKit in Xcode 27's
// place and maestro-runner only in its default folder.
func healthy() fakeMachine {
	return fakeMachine{
		out: map[string]string{
			"xcodebuild -version":           "Xcode 27.0\nBuild version 27A266a\n",
			"xcrun simctl list runtimes -j": runtimesJSON,
			"xcode-select -p":               "/Applications/Xcode.app/Contents/Developer\n",
			"adb version":                   "Android Debug Bridge version 1.0.41\n",
			"emulator -list-avds":           "INFO    | Storing crashdata\ne2e_emulator\nPixel_9\n",
			"node --version":                "v24.13.0\n",
			"claude --version":              "2.1.278 (Claude Code)\n",
			"/Users/dev/.maestro-runner/bin/maestro-runner --version": "maestro-runner 1.1.27\n",
		},
		exists: map[string]bool{"/Applications/Xcode.app/Contents/SharedFrameworks/SimulatorKit.framework": true},
	}
}

func TestRunHealthyMachine(t *testing.T) {
	results := Run(context.Background(), healthy().env())
	want := map[string]string{
		"Xcode": "Xcode 27.0", "iOS runtime": "iOS 26.2", "SimulatorKit": "found",
		"adb": "version 1.0.41", "Android emulator": "e2e_emulator, Pixel_9",
		"Node.js": "v24.13.0", "Claude Code": "2.1.278 (Claude Code)", "maestro-runner": "maestro-runner 1.1.27",
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d", len(results), len(want))
	}
	for _, r := range results {
		if r.Status != OK || r.Found != want[r.Name] {
			t.Errorf("%s = %+v, want OK with %q", r.Name, r, want[r.Name])
		}
	}
	if len(Problems(results)) != 0 {
		t.Error("a healthy machine has no problems")
	}
}

func TestRunBareMachine(t *testing.T) {
	results := Run(context.Background(), fakeMachine{}.env())
	for _, r := range results {
		if r.Status == OK || r.Fix == "" {
			t.Errorf("%s on a bare machine = %+v, want a problem with a fix", r.Name, r)
		}
	}
	byName := map[string]Status{}
	for _, r := range results {
		byName[r.Name] = r.Status
	}
	for name, want := range map[string]Status{
		"Xcode": Missing, "adb": Missing, "Node.js": Optional, "Claude Code": Optional, "maestro-runner": Optional,
	} {
		if byName[name] != want {
			t.Errorf("%s status = %v, want %v", name, byName[name], want)
		}
	}
}

func TestChecksEdgeCases(t *testing.T) {
	m := healthy()
	m.out["xcode-select -p"] = "/Applications/Xcode-26.app/Contents/Developer\n"
	m.exists = map[string]bool{"/Applications/Xcode-26.app/Contents/Developer/Library/PrivateFrameworks/SimulatorKit.framework": true}
	m.out["xcrun simctl list runtimes -j"] = `{"runtimes":[{"name":"iOS 18.6","isAvailable":true}]}`
	m.out["emulator -list-avds"] = "\n"
	m.out["node --version"] = "v16.20.0\n"
	m.onPath = map[string]string{"maestro-runner": "/opt/bin/maestro-runner"}
	m.out["/opt/bin/maestro-runner --version"] = "maestro-runner 1.1.28\n"
	got := map[string]Result{}
	for _, r := range Run(context.Background(), m.env()) {
		got[r.Name] = r
	}
	if got["SimulatorKit"].Status != OK {
		t.Error("Xcode 26 layout not recognized")
	}
	if got["iOS runtime"].Status != Missing {
		t.Error("only iOS 18.6 must not satisfy the runtime floor")
	}
	if got["Android emulator"].Status != Missing {
		t.Error("no AVDs listed must be missing")
	}
	if r := got["Node.js"]; r.Status != Optional || r.Found != "v16.20.0 (too old)" {
		t.Errorf("old Node = %+v", r)
	}
	if r := got["maestro-runner"]; r.Found != "maestro-runner 1.1.28" {
		t.Errorf("maestro-runner on PATH = %+v", r)
	}
}

func TestNodeMajor(t *testing.T) {
	for v, want := range map[string]int{"v24.13.0": 24, "18.0.0": 18, "vX": 0, "": 0} {
		if got := nodeMajor(v); got != want {
			t.Errorf("nodeMajor(%q) = %d, want %d", v, got, want)
		}
	}
}

func TestPrint(t *testing.T) {
	results := Run(context.Background(), fakeMachine{}.env())
	var full, short bytes.Buffer
	Print(&full, results, false)
	PrintProblems(&short, results, false)
	for _, want := range []string{"✗ Xcode", "- Node.js", fixXcode, fixMaestroR} {
		if !strings.Contains(full.String(), want) || !strings.Contains(short.String(), want) {
			t.Errorf("output missing %q:\n%s\n%s", want, full.String(), short.String())
		}
	}
	var styled bytes.Buffer
	Print(&styled, results, true)
	if !strings.Contains(styled.String(), "\x1b[1m✗\x1b[0m") {
		t.Errorf("a missing tool should be bold on a terminal: %q", styled.String())
	}

	var ok bytes.Buffer
	PrintProblems(&ok, Run(context.Background(), healthy().env()), false)
	if !strings.Contains(ok.String(), "all found") {
		t.Errorf("healthy summary = %q", ok.String())
	}
	var okFull bytes.Buffer
	Print(&okFull, Run(context.Background(), healthy().env()), false)
	if !strings.Contains(okFull.String(), "✓ Xcode") {
		t.Errorf("full list = %q", okFull.String())
	}
}

func TestSystemEnv(t *testing.T) {
	env := System()
	if env.Home == "" || env.Run == nil || env.LookPath == nil {
		t.Fatal("System env incomplete")
	}
	if !env.Exists("/") || env.Exists("/definitely/not/here") {
		t.Error("Exists is wrong")
	}
	if out, err := env.Run(context.Background(), "echo", "hi"); err != nil || strings.TrimSpace(string(out)) != "hi" {
		t.Errorf("Run = %q, %v", out, err)
	}
}
