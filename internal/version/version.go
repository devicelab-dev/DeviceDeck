// Package version carries the build-time identity of the devicedeck binary.
// Values are injected via -ldflags at release build; defaults mark a dev build.
package version

import "fmt"

// Set at build time via -ldflags "-X .../internal/version.Version=... -X .../internal/version.Commit=...".
var (
	// Version is the semantic version of this build, or "dev" for local builds.
	Version = "dev"
	// Commit is the short git commit hash the binary was built from.
	Commit = "none"
)

// Line renders the one-line identity string printed by the CLI.
func Line() string {
	return fmt.Sprintf("devicedeck %s (%s)", Version, Commit)
}
