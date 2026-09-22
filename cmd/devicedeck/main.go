// Command devicedeck is the single-binary DeviceDeck server: it streams iOS
// Simulators and Android emulators to a browser, injects input, and exposes
// the UI tree so a manual session can be captured as a replayable flow.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/brand"
	"github.com/devicelab-dev/DeviceDeck/internal/doctor"
	"github.com/devicelab-dev/DeviceDeck/internal/version"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
)

// Coverage waiver: main is the process entry point — it reads os.Args,
// dispatches to the subcommands with real backends, and calls os.Exit; the
// dispatch decision itself (command) and every subcommand's pieces are
// covered directly.
func main() {
	cmd, rest := command(os.Args[1:])
	switch cmd {
	case "serve":
		exitOn("devicedeck", runServe(rest))
	case "mcp":
		exitOn("devicedeck", runMCP(rest))
	case "doctor":
		runDoctor(context.Background(), doctor.System(), os.Stdout)
	case "_video-android":
		// Hidden: Android screen capture, spawned by the video manager
		// against its own binary so no separate sidecar ships.
		exitOn("devicedeck video-android", video.RunAndroidCapture(arg(2), os.Stdin, os.Stdout))
	case "version":
		_, _ = fmt.Fprintln(os.Stdout, version.Line())
	case "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "devicedeck: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

// command picks the subcommand. With none, or only flags, it is serve:
// starting the server is what running devicedeck means, so `devicedeck` and
// `devicedeck --addr :9000` both just work.
func command(args []string) (string, []string) {
	if len(args) == 0 {
		return "serve", nil
	}
	switch args[0] {
	case "version", "--version", "-version", "-v":
		return "version", nil
	case "help", "--help", "-help", "-h":
		return "help", nil
	}
	if strings.HasPrefix(args[0], "-") {
		return "serve", args
	}
	return args[0], args[1:]
}

// exit ends the process; replaced in tests.
var exit = os.Exit

// exitOn prints err and exits non-zero; a nil err returns.
func exitOn(prefix string, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, prefix+":", err)
		exit(1)
	}
}

// runDoctor prints every tool check, for `devicedeck doctor`.
func runDoctor(ctx context.Context, env doctor.Env, w io.Writer) {
	_, _ = fmt.Fprintf(w, "\n  %s\n\n", version.Line())
	doctor.Print(w, doctor.Run(ctx, env), brand.Hyperlinks(os.Stdout))
	_, _ = fmt.Fprintln(w)
}

// arg returns os.Args[i], or "" when it was not given, so the dispatch
// reads the same whether or not a command was typed.
func arg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}

// usage is what someone sees when they run the binary with no idea what
// it does — which, for a tool distributed as a tarball, is most first
// contacts. It names the one command that matters and where to go next.
func usage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `%s - by %s (%s)

Streams iOS Simulators and Android emulators to a browser, and turns a
session driven by hand into a replayable test.

Usage:
  devicedeck [flags]          start the server and console (same as serve)
  devicedeck serve [flags]    start the server and console
  devicedeck mcp [flags]      run the MCP server (stdio) for an AI agent
  devicedeck doctor           check Xcode, adb, Node.js, Claude Code, maestro-runner
  devicedeck version          print the version
  devicedeck help             print this message

Run "devicedeck --help" for this message, "devicedeck serve --help" for the
server's flags. "devicedeck mcp" speaks
the Model Context Protocol on stdin/stdout and drives a running server, so an
agent can list, boot, launch, and inspect devices; point its MCP client at it.

Once running, open the console at the address it prints (by default
http://127.0.0.1:8787) to pick a device. Point your own tests at
/device/{udid} and drive it with ordinary web selectors.

%s: %s
Star us on GitHub: %s
`, version.Line(), brand.Maker, brand.Site, brand.RealRuns, brand.Site, brand.Repo)
}
