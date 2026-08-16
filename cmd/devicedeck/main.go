// Command devicedeck is the single-binary DeviceDeck server: it streams iOS
// Simulators and Android emulators to a browser, injects input, and exposes
// the UI tree so a manual session can be captured as a replayable flow.
package main

import (
	"fmt"
	"os"

	"github.com/devicelab-dev/DeviceDeck/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "devicedeck:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(os.Stdout, version.Line())
}
