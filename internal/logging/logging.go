// Package logging gives every DeviceDeck process a log folder of its own, so
// a problem can be debugged after the fact from files rather than from
// whatever happened to scroll past in a terminal.
//
// A run writes to ~/.devicedeck/logs/<kind>-<timestamp>/:
//
//	devicedeck.log   everything DeviceDeck logs, always at debug level
//	runner.log       the maestro-runner driver's own diagnostics
//	<component>.log  a sidecar's or capture process's stderr, per device
//	console.log      everything printed to stdout/stderr (serve only), which
//	                 catches libraries that print instead of logging
//	crash.log        the Go runtime's report if the process panics
//
// The terminal keeps a readable level (info by default) while the file keeps
// everything, so turning on debug output is never needed to diagnose a run
// that already happened.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// Keep is how many past run folders survive; older ones are deleted when a
// new run starts, so the logs folder cannot grow without bound.
const Keep = 20

// EnvLevel sets the terminal's level ("debug", "info", "warn", "error").
// Unset, the terminal shows warnings and errors only: the startup guide is
// printed directly, and everything else is in the run's files, which
// always record debug.
const EnvLevel = "DEVICEDECK_LOG"

// Run is one process's log folder. Close it on exit to flush and release the
// files.
type Run struct {
	Dir   string
	main  *os.File
	mu    sync.Mutex
	files map[string]*os.File
	// console is set while CaptureConsole is active: the real stdout and
	// stderr, and a function that restores them and drains the copies.
	console *console
}

// console is the state of a CaptureConsole: the original streams and the
// copies draining the pipes that replaced them.
type console struct {
	stdout, stderr *os.File
	pipes          []*os.File
	done           sync.WaitGroup
}

var (
	activeMu sync.Mutex
	active   *Run
	now      = time.Now // replaced in tests to predict the run folder name
	pipe     = os.Pipe  // replaced in tests to fail console capture
)

// Start creates a run folder under home/logs, makes it the process's slog
// default (file at debug, term at termLevel; a nil term logs to file only),
// routes runtime crash reports there, and prunes old runs.
func Start(home, kind string, term io.Writer, termLevel slog.Level) (*Run, error) {
	dir := filepath.Join(home, "logs", kind+"-"+now().Format("20060102-150405.000"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create log folder: %w", err)
	}
	f, err := openLog(filepath.Join(dir, "devicedeck.log"))
	if err != nil {
		return nil, err
	}
	r := &Run{Dir: dir, main: f, files: map[string]*os.File{}}
	handlers := []slog.Handler{slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})}
	if term != nil {
		handlers = append(handlers, slog.NewTextHandler(term, &slog.HandlerOptions{Level: termLevel}))
	}
	slog.SetDefault(slog.New(slog.NewMultiHandler(handlers...)))
	r.captureCrashes()
	prune(filepath.Join(home, "logs"), Keep)
	setActive(r)
	return r, nil
}

// captureCrashes sends the runtime's panic report to crash.log as well as
// stderr. It is best-effort: a failure here must not stop the run.
func (r *Run) captureCrashes() {
	f, err := openLog(r.Path("crash.log"))
	if err != nil {
		slog.Warn("crash log unavailable", "err", err)
		return
	}
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	r.files["crash"] = f
}

// Path is the location of a named file inside the run folder.
func (r *Run) Path(name string) string { return filepath.Join(r.Dir, name) }

// Level parses a terminal level name, defaulting to warn for anything else.
func Level(name string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(name))); err != nil {
		return slog.LevelWarn
	}
	return l
}

// CaptureConsole copies everything the process prints to stdout and stderr
// into console.log, while still showing it in the terminal. Some libraries
// print progress instead of logging it; this is how those lines reach the
// run's files. Not for a process whose stdout is a protocol (mcp).
func (r *Run) CaptureConsole() error {
	f, err := r.file("console")
	if err != nil {
		return err
	}
	c := &console{stdout: os.Stdout, stderr: os.Stderr}
	for _, std := range []**os.File{&os.Stdout, &os.Stderr} {
		pr, pw, err := pipe()
		if err != nil {
			c.restore()
			return fmt.Errorf("capture console: %w", err)
		}
		c.pipes = append(c.pipes, pw)
		c.done.Add(1)
		go func(dst io.Writer) { defer c.done.Done(); _, _ = io.Copy(dst, pr); _ = pr.Close() }(io.MultiWriter(*std, f))
		*std = pw
	}
	r.console = c
	return nil
}

// restore puts the original streams back and waits for the copies to drain.
func (c *console) restore() {
	os.Stdout, os.Stderr = c.stdout, c.stderr
	for _, p := range c.pipes {
		_ = p.Close()
	}
	c.done.Wait()
}

// stderr is the terminal's real stderr, even while the console is captured,
// so child-process output is not copied into console.log twice.
func (r *Run) stderr() *os.File {
	if r.console != nil {
		return r.console.stderr
	}
	return os.Stderr
}

// Close releases every file the run opened and stops it being the target
// for component output.
func (r *Run) Close() error {
	if r.console != nil {
		r.console.restore()
		r.console = nil
	}
	setActive(nil)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.files {
		_ = f.Close()
	}
	r.files = map[string]*os.File{}
	return r.main.Close()
}

// Component returns where a child process should write its stderr: the
// terminal plus <name>.log in the active run folder, or just the terminal
// when no run is active (tests, or a log folder that could not be made).
// The file is opened once per name and shared, so a process restarted for
// the same device appends to the same log.
func Component(name string) io.Writer {
	r := getActive()
	if r == nil {
		return os.Stderr
	}
	f, err := r.file(name)
	if err != nil {
		slog.Warn("component log unavailable", "component", name, "err", err)
		return r.stderr()
	}
	return io.MultiWriter(r.stderr(), f)
}

func (r *Run) file(name string) (*os.File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.files[name]; ok {
		return f, nil
	}
	f, err := openLog(r.Path(safeName(name) + ".log"))
	if err != nil {
		return nil, err
	}
	r.files[name] = f
	return f, nil
}

// safeName keeps a component name usable as a file name: device ids may be
// avd:-prefixed or carry other separators.
func safeName(name string) string {
	return strings.Map(func(c rune) rune {
		if c == '/' || c == ':' || c == os.PathSeparator {
			return '_'
		}
		return c
	}, name)
}

func openLog(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", filepath.Base(path), err)
	}
	return f, nil
}

// prune deletes all but the newest keep run folders. Folder names start
// with a sortable timestamp after the kind prefix, so modification time is
// used instead of the name to order runs of different kinds together.
func prune(logs string, keep int) {
	entries, err := os.ReadDir(logs)
	if err != nil {
		return
	}
	type run struct {
		name string
		mod  time.Time
	}
	var runs []run
	for _, e := range entries {
		if info, err := e.Info(); err == nil && e.IsDir() {
			runs = append(runs, run{e.Name(), info.ModTime()})
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].mod.After(runs[j].mod) })
	for i := keep; i < len(runs); i++ {
		_ = os.RemoveAll(filepath.Join(logs, runs[i].name))
	}
}

func setActive(r *Run) {
	activeMu.Lock()
	active = r
	activeMu.Unlock()
}

func getActive() *Run {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active
}
