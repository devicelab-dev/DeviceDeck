// Command devicedeck is the single-binary DeviceDeck server: it streams iOS
// Simulators and Android emulators to a browser, injects input, and exposes
// the UI tree so a manual session can be captured as a replayable flow.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/devicelab-dev/DeviceDeck/internal/version"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
)

// Coverage waiver: main is the process entry point — it reads os.Args,
// dispatches to runServe/RunAndroidCapture with real backends, and calls
// os.Exit; the dispatch's testable pieces (arg, usage, version.Line) are
// covered directly.
func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "devicedeck:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := runMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "devicedeck:", err)
			os.Exit(1)
		}
		return
	}
	// Hidden subcommand: Android screen capture, spawned by the video
	// manager against its own binary so no separate sidecar ships.
	if len(os.Args) > 2 && os.Args[1] == "_video-android" {
		if err := video.RunAndroidCapture(os.Args[2], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "devicedeck video-android:", err)
			os.Exit(1)
		}
		return
	}
	switch arg(1) {
	case "version", "--version", "-version", "-v":
		fmt.Fprintln(os.Stdout, version.Line())
	case "", "help", "--help", "-help", "-h":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "devicedeck: unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
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
	fmt.Fprintf(w, `%s

Streams iOS Simulators and Android emulators to a browser, and turns a
session driven by hand into a replayable test.

Usage:
  devicedeck serve [flags]    start the server and console
  devicedeck mcp [flags]      run the MCP server (stdio) for an AI agent
  devicedeck version          print the version
  devicedeck help             print this message

Run "devicedeck serve --help" for the server's flags. "devicedeck mcp" speaks
the Model Context Protocol on stdin/stdout and drives a running server, so an
agent can list, boot, launch, and inspect devices; point its MCP client at it.

Once running, open the console at the address it prints (by default
http://127.0.0.1:8787) to pick a device. Point your own tests at
/device/{udid} and drive it with ordinary web selectors.
`, version.Line())
}
