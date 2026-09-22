package doctor

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/ready"
)

// Where each fix points. Kept together so the wording stays consistent.
const (
	fixXcode    = "install Xcode from the App Store, then: sudo xcode-select -s /Applications/Xcode.app"
	fixRuntime  = "install the iOS 26.2 (or newer) simulator runtime: Xcode > Settings > Components"
	fixKit      = "reinstall or update Xcode; touch and keyboard input on iOS need its SimulatorKit"
	fixADB      = "install Android platform-tools (Android Studio > SDK Manager) and put adb on PATH"
	fixAVD      = "create an emulator in Android Studio > Device Manager, and put emulator on PATH"
	fixNode     = "install Node.js 18 or newer (https://nodejs.org); Playwright MCP runs through npx"
	fixClaude   = "optional: npm install -g @anthropic-ai/claude-code, to drive devices from Claude Code"
	fixMaestroR = "optional, to replay captured flows locally: curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash"
)

// firstLine runs a command and returns its first output line, trimmed.
func firstLine(ctx context.Context, env Env, name string, args ...string) (string, bool) {
	out, err := env.Run(ctx, name, args...)
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line), line != ""
}

func checkXcode(ctx context.Context, env Env) Result {
	r := Result{Name: "Xcode", For: "iOS simulators"}
	if v, ok := firstLine(ctx, env, "xcodebuild", "-version"); ok {
		r.Found = v
		return r
	}
	r.Status, r.Fix = Missing, fixXcode
	return r
}

func checkIOSRuntime(ctx context.Context, env Env) Result {
	r := Result{Name: "iOS runtime", For: "a stable iOS simulator (26.2 or newer)"}
	out, err := env.Run(ctx, "xcrun", "simctl", "list", "runtimes", "-j")
	var payload struct {
		Runtimes []struct {
			Name        string `json:"name"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"runtimes"`
	}
	if err == nil && json.Unmarshal(out, &payload) == nil {
		for _, rt := range payload.Runtimes {
			if rt.IsAvailable && ready.MeetsFloor(rt.Name) {
				r.Found = rt.Name
			}
		}
	}
	if r.Found == "" {
		r.Status, r.Fix = Missing, fixRuntime
	}
	return r
}

// checkSimulatorKit looks where Xcode 26 and Xcode 27 keep SimulatorKit,
// the framework the input sidecar loads.
func checkSimulatorKit(ctx context.Context, env Env) Result {
	r := Result{Name: "SimulatorKit", For: "touch and keyboard input on iOS"}
	dev, ok := firstLine(ctx, env, "xcode-select", "-p")
	if ok {
		for _, p := range []string{
			filepath.Join(dev, "Library/PrivateFrameworks/SimulatorKit.framework"),
			filepath.Join(filepath.Dir(dev), "SharedFrameworks/SimulatorKit.framework"),
		} {
			if env.Exists(p) {
				r.Found = "found"
				return r
			}
		}
	}
	r.Status, r.Fix = Missing, fixKit
	return r
}

func checkADB(ctx context.Context, env Env) Result {
	r := Result{Name: "adb", For: "Android emulators"}
	if v, ok := firstLine(ctx, env, "adb", "version"); ok {
		r.Found = strings.TrimPrefix(v, "Android Debug Bridge ")
		return r
	}
	r.Status, r.Fix = Missing, fixADB
	return r
}

func checkAVD(ctx context.Context, env Env) Result {
	r := Result{Name: "Android emulator", For: "booting an Android device"}
	out, err := env.Run(ctx, "emulator", "-list-avds")
	var avds []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "INFO") {
			avds = append(avds, l)
		}
	}
	if err != nil || len(avds) == 0 {
		r.Status, r.Fix = Missing, fixAVD
		return r
	}
	r.Found = strings.Join(avds, ", ")
	return r
}

func checkNode(ctx context.Context, env Env) Result {
	r := Result{Name: "Node.js", For: "Playwright MCP and Playwright tests"}
	v, ok := firstLine(ctx, env, "node", "--version")
	if major := nodeMajor(v); ok && major >= 18 {
		r.Found = v
		return r
	}
	r.Status, r.Fix = Optional, fixNode
	if ok {
		r.Found = v + " (too old)"
	}
	return r
}

// nodeMajor reads "v22.3.0" as 22; anything unreadable is 0.
func nodeMajor(v string) int {
	major, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), ".")
	n := 0
	for _, c := range major {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func checkClaude(ctx context.Context, env Env) Result {
	r := Result{Name: "Claude Code", For: "driving devices from Claude"}
	if v, ok := firstLine(ctx, env, "claude", "--version"); ok {
		r.Found = v
		return r
	}
	r.Status, r.Fix = Optional, fixClaude
	return r
}

// checkMaestroRunner finds maestro-runner on PATH or in its default install
// folder. DeviceDeck does not need it; it replays captured flows locally.
func checkMaestroRunner(ctx context.Context, env Env) Result {
	r := Result{Name: "maestro-runner", For: "replaying captured flows locally"}
	bin, err := env.LookPath("maestro-runner")
	if err != nil {
		bin = filepath.Join(env.Home, ".maestro-runner", "bin", "maestro-runner")
	}
	if v, ok := firstLine(ctx, env, bin, "--version"); ok {
		r.Found = v
		return r
	}
	r.Status, r.Fix = Optional, fixMaestroR
	return r
}
