package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/devicelab-dev/DeviceDeck/internal/apps"
	"github.com/devicelab-dev/DeviceDeck/internal/brand"
	"github.com/devicelab-dev/DeviceDeck/internal/capture"
	"github.com/devicelab-dev/DeviceDeck/internal/doctor"
	"github.com/devicelab-dev/DeviceDeck/internal/emu"
	"github.com/devicelab-dev/DeviceDeck/internal/home"
	"github.com/devicelab-dev/DeviceDeck/internal/input"
	"github.com/devicelab-dev/DeviceDeck/internal/logging"
	"github.com/devicelab-dev/DeviceDeck/internal/platform"
	"github.com/devicelab-dev/DeviceDeck/internal/ready"
	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/server"
	"github.com/devicelab-dev/DeviceDeck/internal/sim"
	"github.com/devicelab-dev/DeviceDeck/internal/version"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
	"github.com/devicelab-dev/DeviceDeck/internal/web"
)

// serveFlags is what `serve` was asked to do, parsed once so the wiring in
// runServe reads as wiring.
type serveFlags struct {
	addr, hidPath, videoPath string
	apps                     []string // --app, repeatable
	fps                      int
	keepDevices, ready       bool
}

// readyApp is what --ready works with: the first --app, or none.
func (f *serveFlags) readyApp() string {
	if len(f.apps) == 0 {
		return ""
	}
	return f.apps[0]
}

// parseServeFlags parses serve's arguments into serveFlags.
func parseServeFlags(args []string) (*serveFlags, error) {
	f := &serveFlags{}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.StringVar(&f.addr, "addr", "0.0.0.0:8787",
		"listen address; the default accepts other machines on your network, 127.0.0.1:8787 keeps it to this Mac")
	flags.StringVar(&f.hidPath, "sidecar", "", "path to devicedeck-hid (default: auto-discover)")
	flags.StringVar(&f.videoPath, "video-sidecar", "", "path to devicedeck-video (default: auto-discover)")
	flags.IntVar(&f.fps, "fps", 30, "video capture frame rate")
	flags.BoolVar(&f.keepDevices, "keep-devices", false,
		"leave the Android emulators DeviceDeck started running on exit (iOS simulators are shut down by the test runner regardless)")
	flags.BoolVar(&f.ready, "ready", false,
		"resolve a device (prefer booted, else newest iOS runtime >= 26.2), bring it up, and print a machine-readable status object")
	flags.Func("app", "an app build (.app for iOS, .apk for Android) to install on a device the first "+
		"time that app is launched there; repeat for both platforms. With --ready, a bundle id also works",
		func(v string) error { f.apps = append(f.apps, v); return nil })
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	for _, a := range f.apps {
		if !f.ready && !isAppFile(a) {
			return nil, fmt.Errorf("--app %q: pass a .app or .apk path (a bundle id works only with --ready)", a)
		}
	}
	return f, nil
}

// runServe starts the DeviceDeck HTTP server and blocks until SIGINT/SIGTERM.
//
// Coverage waiver: runServe is process-lifecycle wiring (real listener,
// signals, real backends) verified by running the server; unit tests cover
// every step it calls: parseServeFlags, setupServe, buildStack, bringReady,
// waitForExit and shutdown.
func runServe(args []string) error {
	opts, err := parseServeFlags(args)
	if err != nil {
		return err
	}
	env, err := setupServe(opts)
	if err != nil {
		return err
	}
	defer env.close()
	st, err := buildStack(env.hid, env.video, opts.fps, opts.apps)
	if err != nil {
		return err
	}
	httpServer := &http.Server{Addr: opts.addr, Handler: st.srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errCh, err := listen(httpServer)
	if err != nil {
		return err
	}
	greet(st, env, opts, doctor.System(), brand.UpdateURL, os.Stderr)
	if err := waitForExit(errCh); err != nil {
		slog.Error("server stopped", "err", err)
		return err
	}
	shutdown(st, httpServer, opts.keepDevices)
	brand.Footer(os.Stderr, env.hyper)
	return nil
}

// serveEnv is what serve needs before it can build the stack: the sidecar
// binaries, and the run's log folder so everything after setup is recorded.
type serveEnv struct {
	hid, video string
	logs       *logging.Run
	hyper      bool // the terminal takes clickable links
}

// close flushes the run's logs, the runner's driver log included.
func (e *serveEnv) close() {
	runner.CloseLogFile()
	_ = e.logs.Close()
}

// setupServe starts the run's logs first, so a failure in any later step
// (preparing the home folder, finding a sidecar) is recorded in them too.
func setupServe(opts *serveFlags) (*serveEnv, error) {
	// Decided before the console is captured, while stderr is still the
	// terminal itself.
	hyper := brand.Hyperlinks(os.Stderr)
	brand.Banner(os.Stderr, version.Line(), hyper)
	dir, err := home.Dir()
	if err != nil {
		return nil, err
	}
	logs, err := startRunLogs(dir, "serve", os.Stderr)
	if err != nil {
		return nil, err
	}
	env := &serveEnv{logs: logs, hyper: hyper}
	captureConsole(logs)
	attachRunnerLog(logs.Path("runner.log"))
	if err := env.resolve(dir, opts); err != nil {
		slog.Error("devicedeck could not start", "err", err)
		env.close()
		return nil, err
	}
	slog.Debug("devicedeck starting", "version", version.Line(), "home", dir, "logs", logs.Dir,
		"hid", env.hid, "video", env.video)
	return env, nil
}

// resolve prepares the home folder and finds both sidecar binaries.
func (e *serveEnv) resolve(dir string, opts *serveFlags) (err error) {
	if err := prepareHome(dir); err != nil {
		return err
	}
	if e.hid, err = resolveBinary("devicedeck-hid", opts.hidPath); err != nil {
		return err
	}
	e.video, err = resolveBinary("devicedeck-video", opts.videoPath)
	return err
}

// startRunLogs opens this run's log folder under the DeviceDeck home. The
// terminal level comes from DEVICEDECK_LOG; the files record everything.
func startRunLogs(dir, kind string, term io.Writer) (*logging.Run, error) {
	return logging.Start(dir, kind, term, logging.Level(os.Getenv(logging.EnvLevel)))
}

// announceUpdate tells the user when a newer DeviceDeck is released. It runs
// in the background and says nothing when the check fails or the build is a
// local one, so it never delays or clutters a start.
func announceUpdate(ctx context.Context, client *http.Client, url, current string, w io.Writer) {
	latest, err := brand.Latest(ctx, client, url)
	if err != nil {
		slog.Debug("update check skipped", "err", err)
		return
	}
	if brand.Newer(latest, current) {
		slog.Info("update available", "current", current, "latest", latest)
		brand.UpdateNotice(w, current, latest)
	}
}

// consoleCapturer is the part of a log run that copies stdout and stderr.
type consoleCapturer interface{ CaptureConsole() error }

// captureConsole copies serve's printed output into the run's files; losing
// that is worth a warning, not a failed start.
func captureConsole(r consoleCapturer) {
	if err := r.CaptureConsole(); err != nil {
		slog.Warn("console output will not be logged", "err", err)
	}
}

// attachRunnerLog sends the maestro-runner driver's diagnostics to path.
// Losing them is worth a warning, not a failed start.
func attachRunnerLog(path string) {
	if err := runner.SetLogFile(path); err != nil {
		slog.Warn("runner log unavailable", "err", err)
	}
}

// shutdown stops serving, then ends every sidecar session and engine, and
// powers off the Android emulators DeviceDeck drove unless keep is set.
func shutdown(st *stack, httpServer *http.Server, keep bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	st.inputs.CloseAll()
	st.videos.CloseAll()
	// Capture the driven devices before StopAll clears them; the Android
	// ones are DeviceDeck's to power off.
	driven := st.engines.ActiveUDIDs()
	st.engines.StopAll(ctx)
	if !keep {
		// A deadline of its own: a slow engine stop (a runner relaunching
		// after its simulator vanished) must not use up the time needed to
		// power the emulators off, or they are left running.
		offCtx, offCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer offCancel()
		powerOffAndroid(offCtx, driven, st.emu)
	}
	slog.Info("devicedeck stopped")
}

// localURL is the address to open on this Mac. A wildcard bind (0.0.0.0,
// ::, or no host) is reached through loopback.
func localURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// interfaceAddrs lists this machine's addresses; replaced in tests.
var interfaceAddrs = net.InterfaceAddrs

// networkURLs lists the addresses other machines can use, which only exist
// for a wildcard bind. Loopback and link-local addresses are left out.
func networkURLs(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); err != nil || (host != "" && (ip == nil || !ip.IsUnspecified())) {
		return nil
	}
	addrs, _ := interfaceAddrs()
	var urls []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		urls = append(urls, "http://"+net.JoinHostPort(ipnet.IP.String(), port))
	}
	return urls
}

// stack is everything serve wires together and must tear down again: the
// sidecar managers, the tree engines, the platform clients and the routers
// that pick a client per device. --ready reuses the routers so it drives a
// device exactly the way the HTTP API would.
type stack struct {
	inputs   *input.Manager
	videos   *video.Manager
	engines  *runner.Engines
	emu      *emu.Client
	devices  server.MultiLister
	boots    server.BootRouter
	launches server.LaunchRouter
	catalog  *apps.Catalog
	srv      *server.Server
}

// appFiles keeps the --app values that are build paths; a bundle id is only
// meaningful to --ready.
func appFiles(args []string) []string {
	var files []string
	for _, a := range args {
		if isAppFile(a) {
			files = append(files, a)
		}
	}
	return files
}

// appDevice is how the app catalog reaches devices: check, then install.
type appDevice struct {
	server.InstalledRouter
	server.LaunchRouter
}

// androidInjector adapts the engines' concrete Android engine to the input
// router's injector interface; Go will not convert the method value itself.
//
// Coverage waiver: the only way through is starting a real Android engine
// (adb + the devicelab driver), which is exercised end-to-end, not in unit
// tests.
func (st *stack) androidInjector(ctx context.Context, udid string) (input.AndroidInjector, error) {
	return st.engines.AndroidInjector(ctx, udid)
}

// buildStack constructs the stack without starting any process: the
// managers spawn their sidecars lazily, on the first device that needs one.
// The --app builds are identified here, so a bad path stops the start.
func buildStack(hidBin, videoBin string, fps int, appArgs []string) (*stack, error) {
	st := &stack{
		inputs:  input.NewManager(hidBin),
		videos:  video.NewManager(videoBin, fps),
		engines: runner.NewEngines(),
		emu:     emu.NewClient(),
	}
	simClient := sim.NewClient()
	st.devices = server.MultiLister{simClient, st.emu}
	st.boots = server.BootRouter{IOS: simClient, Android: st.emu}
	st.launches = server.LaunchRouter{IOS: simClient, Android: st.emu}
	// Android input rides the tree engine's driver session; the router
	// picks the backend per device.
	frames := input.NewRouter(st.inputs, st.androidInjector)
	st.srv = server.New(st.devices, st.boots, st.launches,
		server.ScreenshotRouter{IOS: simClient, Android: st.emu},
		frames, st.engines, st.videos, capture.NewService(st.engines))
	st.srv.SetConsole(web.Handler(st.srv.FirstTree))
	st.srv.SetSessionEnder(sessionEnder{engines: st.engines, videos: st.videos, inputs: st.inputs, ios: simClient, android: st.emu})
	st.srv.SetEngineWarmer(bootThenWarm{android: st.emu, engines: st.engines},
		engineDetail(runnerCacheDir(), st.devices, st.emu))
	cat, err := apps.NewCatalog(appFiles(appArgs), appDevice{server.InstalledRouter{IOS: simClient, Android: st.emu}, st.launches})
	if err != nil {
		return nil, err
	}
	st.catalog = cat
	st.srv.SetApps(cat)
	return st, nil
}

// greet runs once the server is listening: the startup guide, the update
// check in the background, and --ready. tools and updateURL are parameters so
// tests can stand in for the machine and the network.
func greet(st *stack, env *serveEnv, opts *serveFlags, tools doctor.Env, updateURL string, out io.Writer) {
	ctx := context.Background()
	local, network := localURL(opts.addr), networkURLs(opts.addr)
	slog.Debug("devicedeck serving", "addr", opts.addr, "console", local, "network", network)
	newWelcome(ctx, st.devices, tools, st.catalog, local, network, env.logs.Dir, env.hyper).write(out)
	go announceUpdate(ctx, http.DefaultClient, updateURL, version.Version, out)
	if opts.ready {
		bringReady(ctx, st.devices, st.boots, st.launches, local, opts.readyApp(), os.Stdout)
	}
}

// listen binds the address before anything says the server is up, so a port
// already in use is a clear error instead of a welcome for a server that is
// not there.
func listen(srv *http.Server) (<-chan error, error) {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		slog.Error("devicedeck could not start", "addr", srv.Addr, "err", err)
		return nil, fmt.Errorf("listen on %s: %w (another devicedeck may be running; use --addr for another port)", srv.Addr, err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	return errCh, nil
}

// waitForExit blocks until the listener fails or SIGINT/SIGTERM arrives. Only
// the listener failure is an error: a signal is the normal way out.
func waitForExit(errCh <-chan error) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
		return nil
	}
}

// sessionEnder is End session: it stops what DeviceDeck runs for a device,
// then powers the device off. The engine goes first: on iOS stopping it can
// shut the simulator down already, which the shutdown then accepts.
type sessionEnder struct {
	engines interface {
		Stop(ctx context.Context, udid string)
	}
	videos interface{ Close(udid string) }
	inputs interface{ Drop(udid string) }
	ios    interface {
		Shutdown(ctx context.Context, udid string) error
	}
	android androidKiller
}

// End stops udid's engine and sidecars and powers the device off.
func (e sessionEnder) End(ctx context.Context, udid string) error {
	e.engines.Stop(ctx, udid)
	e.videos.Close(udid)
	e.inputs.Drop(udid)
	if platform.IsAndroidSerial(udid) {
		return e.android.Kill(ctx, udid)
	}
	return e.ios.Shutdown(ctx, udid)
}

// androidKiller powers off an emulator DeviceDeck started.
type androidKiller interface {
	Kill(ctx context.Context, serial string) error
}

// powerOffAndroid kills the Android emulators among the driven devices.
// Stopping an iOS engine shuts its simulator down — killing xcodebuild tears
// the test session down with it — so the runner already cleans those up.
// Android emulators are detached and outlive that, so they are the ones
// DeviceDeck must power off itself. --keep-devices skips this; iOS is the
// runner's either way.
func powerOffAndroid(ctx context.Context, driven []string, emus androidKiller) {
	for _, udid := range driven {
		if !platform.IsAndroidSerial(udid) {
			continue
		}
		if err := emus.Kill(ctx, udid); err != nil {
			slog.Warn("emulator shutdown on exit", "serial", udid, "err", err)
		}
	}
}

// prepareHome gives the runner packages DeviceDeck's own home folder, with
// the Android driver written into it, before anything resolves a driver or a
// build cache. DeviceDeck never borrows another tool's install.
func prepareHome(dir string) error {
	if err := home.Prepare(dir); err != nil {
		return fmt.Errorf("prepare %s: %w", dir, err)
	}
	slog.Debug("devicedeck home", "dir", dir)
	return nil
}

// resolveBinary finds a sidecar binary: explicit flag value, then the
// DEVICEDECK_<NAME> env var, then next to this executable, then the local
// Swift build output (developer setup).
func resolveBinary(name, explicit string) (string, error) {
	envVar := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) // devicedeck-hid → DEVICEDECK_HID
	// A path given deliberately must be honoured or refused, never
	// quietly replaced: falling through to a different binary than the
	// one someone named turns a typo into a mystery, and it would run
	// the wrong build without ever saying so.
	for _, named := range []struct{ source, path string }{
		{"--sidecar/--video-sidecar", explicit},
		{envVar, os.Getenv(envVar)},
	} {
		if named.path == "" {
			continue
		}
		if info, err := os.Stat(named.path); err != nil || info.IsDir() {
			return "", fmt.Errorf("%s points at %q, which is not an executable file (%s)",
				named.source, named.path, name)
		}
		return named.path, nil
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	candidates = append(candidates, filepath.Join("sidecar/.build/release", name))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s not found; build it with `make sidecar` or pass a flag (env %s also works)", name, envVar)
}

// bringReady resolves the device `--ready` should serve, boots it if needed,
// optionally installs and launches an app, and writes one status object an
// agent can parse to decide it may start. The decision is ready.ResolveDevice;
// this carries it out through the same platform routers the server uses, so
// a booted emulator is driven by adb and a simulator by simctl.
func bringReady(ctx context.Context, devices server.DeviceLister, boots server.DeviceBooter,
	launches server.AppLauncher, baseURL, app string, out io.Writer,
) {
	chosen, reason, ok := ready.ResolveDevice(readyDevices(ctx, devices))
	var warnings []string
	appInstalled := false
	if ok && !chosen.Booted {
		if err := boots.Boot(ctx, chosen.UDID); err != nil {
			warnings = append(warnings, "boot: "+err.Error())
		}
	}
	if ok && app != "" {
		appInstalled = readyLaunch(ctx, launches, chosen.UDID, app, &warnings)
	}
	status := ready.BuildStatus(baseURL, chosen, reason, ok, readyAppID(app), appInstalled, warnings)
	b, _ := json.Marshal(status) // plain strings and bools: cannot fail
	_, _ = fmt.Fprintln(out, string(b))
}

// readyDevices lists every simulator and emulator as the resolver sees them.
// A listing failure is not fatal: the resolver then reports that nothing
// qualifies, which is the honest status when no source answered.
func readyDevices(ctx context.Context, devices server.DeviceLister) []ready.Device {
	all, err := devices.All(ctx)
	if err != nil {
		return nil
	}
	out := make([]ready.Device, 0, len(all))
	for _, d := range all {
		out = append(out, ready.Device{UDID: d.UDID, Name: d.Name, OS: d.OS, Booted: d.Booted})
	}
	return out
}

// readyLaunch carries out the --app step: a file path is installed (the
// build's own bundle id is not known here, so launch it separately by id), a
// bundle id is launched. Errors become warnings so serve still comes up with
// the device even when the app step fails. Returns whether the app is now on
// the device.
func readyLaunch(ctx context.Context, launches server.AppLauncher, udid, app string, warnings *[]string) bool {
	if isAppFile(app) {
		if err := launches.Install(ctx, udid, app); err != nil {
			*warnings = append(*warnings, "install: "+err.Error())
			return false
		}
		*warnings = append(*warnings, "installed the build; launch it by bundle id (--app <id>) to bring it to the foreground")
		return true
	}
	if err := launches.LaunchApp(ctx, udid, app); err != nil {
		*warnings = append(*warnings, "launch: "+err.Error())
	}
	return true
}

// isAppFile reports whether --app names a build to install rather than a
// bundle id to launch.
func isAppFile(app string) bool {
	if strings.HasSuffix(app, ".app") || strings.HasSuffix(app, ".apk") || strings.Contains(app, "/") {
		return true
	}
	_, err := os.Stat(app) // a folder of builds named without a slash
	return err == nil
}

// readyAppID is the bundle id label for the status: a real bundle id passes
// through, a file path has none to report.
func readyAppID(app string) string {
	if isAppFile(app) {
		return ""
	}
	return app
}
