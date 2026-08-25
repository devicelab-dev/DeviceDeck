---
name: devicedeck-authoring
description: Use when authoring a mobile UI test or flow against an iOS simulator or Android emulator through DeviceDeck — picking a device, launching an app fresh, and driving it by durable selector so the test is reviewable and runs on real hardware unchanged.
metadata:
  short-description: Author a reviewable mobile UI test with DeviceDeck
---

# Authoring mobile tests with DeviceDeck

DeviceDeck mirrors a simulator/emulator's native UI tree as real DOM, so you drive it
by accessibility identifier — a durable selector, never a coordinate. What you author
here runs unchanged on real hardware (that is the whole point).

**Prerequisite:** a `devicedeck serve` is running (default `http://127.0.0.1:8787`). If a
tool reports a connection error, start it in a terminal first.

## Workflow

1. **Find a device** — `mcp__devicedeck__list_devices`. Note the `udid` (iOS) or serial (Android).
2. **Boot it if needed** — `mcp__devicedeck__boot_device`.
3. **Launch the app fresh** — `mcp__devicedeck__launch_app` with the bundle id / package. It
   wipes app data by default, starting at a first-run screen (pass `fresh: false` to resume).
4. **See what's on screen** — `mcp__devicedeck__ui_tree` for structure, or
   `mcp__devicedeck__screenshot` when you need pixels. Choose elements by their `identifier`.
5. **Act by selector** — `mcp__devicedeck__tap` with the element's `testid`, and
   `mcp__devicedeck__assert_visible` (by testid or visible text) to check each step landed.
6. **Type** — the MCP does not type (keeping it reliable). Get the page with
   `mcp__devicedeck__device_page_url`, open it with your browser tools (Playwright/Puppeteer),
   and `fill()`/type there — they verify each keystroke against the device's read-back.
7. **Emit the test** — write it in the caller's framework using the same `data-testid` selectors
   (`getByTestId('login-button')`). Assert against the device tree, not a guess.

## Rules that keep a test durable

- **Selectors, never coordinates.** Tap and assert by identifier; a captured flow that used
  pixels reviews as noise and breaks on the first layout change.
- **Wait for the device to echo a typed value before submitting.** Keys land a moment after the
  DOM has them; a submit fired too early acts on a half-typed form.
- **Reset between independent tests.** A native app stays logged in across relaunch; `launch_app`
  fresh (the default) is the clean slate.
- **One driver per device.** A device serves one client at a time; run with a single worker.
