// Package runner is the single seam between DeviceDeck and maestro-runner
// (github.com/devicelab-dev/maestro-runner).
//
// maestro-runner is an internal backend detail, never a framework imposed on
// users. DeviceDeck's user-facing surface is protocol (HTTP/WS + a
// WebDriver/Appium-shaped endpoint) so teams keep whatever framework they
// already run; capture exports to the user's chosen format via pluggable
// exporters.
//
// Contract:
//   - Only this package may import maestro-runner, and only pkg/driver (the
//     devicelab drivers — the XCUITest-backed source of the UI tree) and
//     pkg/flow (YAML types, so the Maestro exporter emits the exact format
//     devicelab.dev also runs). No executor, no report, no CLI.
//   - Only the devicelab drivers are used (devicelab_ios and the Android
//     counterpart). WDA and other drivers are out of scope by decision.
//   - The runner is imported as a library, never shelled out to.
package runner
