// Package runner is the single seam between DeviceDeck and maestro-runner
// (github.com/devicelab-dev/maestro-runner).
//
// maestro-runner is an internal backend detail, never a framework imposed on
// users. DeviceDeck's user-facing surfaces are the HTTP/WS API and the
// console's DOM mirror — a positioned overlay of the native tree that web
// tooling (Playwright, CDP, Cypress) automates with real selectors, the
// browser itself providing the protocol. Capture exports to the user's
// chosen framework via pluggable exporters, Maestro YAML first.
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
