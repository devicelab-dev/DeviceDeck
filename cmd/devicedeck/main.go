// Command devicedeck is the single-binary DeviceDeck server: it streams iOS
// Simulators and Android emulators to a browser, injects input, and exposes
// the UI tree so a manual session can be captured as a replayable flow.
package main

import (
	"fmt"
	"os"

	"github.com/devicelab-dev/DeviceDeck/internal/version"
	"github.com/devicelab-dev/DeviceDeck/internal/video"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := runServe(os.Args[2:]); err != nil {
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
	fmt.Fprintln(os.Stdout, version.Line())
}
