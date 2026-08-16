// Package runner is the single seam between DeviceDeck and maestro-runner
// (github.com/devicelab-dev/maestro-runner).
//
// Contract:
//   - Only this package may import maestro-runner. Everything else in
//     DeviceDeck talks to types defined here, so a runner refactor breaks
//     exactly one package.
//   - Only the devicelab drivers are used (devicelab_ios and the Android
//     counterpart). WDA and other drivers are out of scope by decision
//     (PROJECT-BRIEF.md §11.6) — do not add code paths for them.
//   - The runner is imported as a library, never shelled out to.
package runner
