// Package doctor checks the tools DeviceDeck and its users rely on (Xcode,
// the iOS runtime, adb, an Android emulator, Node.js for Playwright, Claude
// Code, maestro-runner) and says how to fix what is missing.
//
// None of it blocks the server: a Mac with only Xcode still serves iOS, and
// one with only the Android SDK still serves Android. The checks turn "why
// is nothing listed" into a line naming the missing piece and the command
// that installs it.
package doctor

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Status is how a check came out.
type Status int

// A check passes, finds something missing a platform needs, or finds an
// optional tool absent.
const (
	OK Status = iota
	Missing
	Optional
)

// Result is one check's outcome: what was found (or not), what it is for,
// and how to fix it when it is not OK.
type Result struct {
	Name   string
	Status Status
	Found  string
	For    string
	Fix    string
}

// Env is how checks reach the machine; tests replace every part of it.
type Env struct {
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath func(file string) (string, error)
	Exists   func(path string) bool
	Home     string
}

// System is the real machine.
func System() Env {
	home, _ := os.UserHomeDir()
	return Env{
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		LookPath: exec.LookPath,
		Exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		Home: home,
	}
}

// checkTimeout bounds each check, so one hung tool cannot hold up a start.
const checkTimeout = 5 * time.Second

// check is one probe of the machine.
type check func(ctx context.Context, env Env) Result

// all is every check, in the order they are shown.
var all = []check{
	checkXcode, checkIOSRuntime, checkSimulatorKit,
	checkADB, checkAVD,
	checkNode, checkClaude, checkMaestroRunner,
}

// Run performs every check concurrently and returns results in display order.
func Run(ctx context.Context, env Env) []Result {
	results := make([]Result, len(all))
	var wg sync.WaitGroup
	for i, c := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, checkTimeout)
			defer cancel()
			results[i] = c(cctx, env)
		}()
	}
	wg.Wait()
	return results
}

// Problems is the results that are not OK, for a short startup summary.
func Problems(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if r.Status != OK {
			out = append(out, r)
		}
	}
	return out
}
