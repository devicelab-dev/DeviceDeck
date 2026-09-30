# Changelog

What changed in each DeviceDeck release. Dates are release dates.

## 1.1.0 — 2026-10-01

### Added

- **New device drivers.** iOS simulators run on DeviceLab's new iOS agent, which ships inside
  DeviceDeck and stays running across restarts; Android emulators run on the updated DeviceLab
  Android driver.
- **Xcode 27 support.** Touch and keys reach Xcode 27 simulators through their new input service.
- **Device settings:** dark mode, GPS location, app permissions and deep links — in the console's
  Settings panel, the device page URL (`?appearance=`, `?location=`, `?grant=`, `?revoke=`,
  `?link=`), `POST /api/devices/{udid}/settings`, and the MCP `device_settings` tool.
- **MCP tools** `fill`, `wait_for` and `device_settings`; `tap` and `long_press` take a snapshot
  ref, visible text, or role and name; `snapshot` has a `diff` mode.
- **Access token** for shared machines: `devicedeck --token` / `DEVICEDECK_TOKEN`, and
  `devicedeck mcp --token`.
- **`devicedeck-maestro` skill:** an agent writes a Maestro flow with DeviceDeck and runs it with
  maestro-runner.
- **Playwright recipes:** accessibility check (axe), permission prompts, two devices in one test,
  a seed test for Playwright's test agents.
- **Android H.264 video**, opt-in with `DEVICEDECK_ANDROID_CAPTURE=h264`.
- `#mirror[data-dd-ready]` on the device page, set once the URL's setup is applied.
- `DEVICEDECK_NO_UPDATE_CHECK` turns off the startup update check.

### Changed

- A launch waits for the device to finish booting and starts the UI engine before the app.
- On exit DeviceDeck powers off the simulators and emulators it drove (`--keep-devices` keeps them).
- MCP screenshots are JPEG at most 1568 px (`full=true` for the original PNG); large results are
  saved to a file; calls run in parallel with progress on long boots.
- MCP refuses a target that is off screen, under the keyboard or under Android's status bar,
  instead of reporting a tap that missed.
- iOS video is captured when the screen changes, not polled.
- Recordings made on Android close the keyboard before a tap that follows typing.

### Fixed

- A launch right after a boot could fail after minutes (iOS agent) or at once (Android install).
- Android passwords were written into captured flows in plain text.
- Typed text in a recording could be split into two steps (a password would be typed twice).
- Android permission grants reported success for permissions the app does not declare.
- Several elements could share one MCP snapshot ref; Android buttons had no name in snapshots.
- Device commands were killed half-way when the client that sent them disconnected.
- iOS system alerts (permission prompts) were missing from the tree.

### Known issues

- iOS system alerts in landscape report their buttons in portrait coordinates.
- Two-finger gestures are dropped on Android.

## 1.0.1 — 2026-09-23

- Device page: every action waits for the device to act; `fill()` reads the value back.
- MCP `list_apps`, so an agent finds "my app" itself; `--app` registers builds that install on
  first launch.
- Console: device library, Apps, End session, Inspect that follows the screen, clear fault cards.
- npm package (`npm install devicedeck`); signed and notarized macOS builds for both architectures.
- Setup for Codex, Cursor, Gemini CLI and VS Code as well as Claude Code; getting-started and CLI
  reference docs.

## 0.1.0 — 2026-08-19

- First release: iOS simulators and Android emulators served as real-DOM web pages, driven by
  Playwright and AI agents; the console; Flow Capture to Maestro YAML.
