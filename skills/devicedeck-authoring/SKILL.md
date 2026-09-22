---
name: devicedeck-authoring
description: Use when authoring a Playwright test against an iOS simulator or Android emulator through DeviceDeck. The device is a real-DOM web page — drive it with Playwright MCP and emit a Playwright spec by durable selector, reviewable and runnable on real hardware unchanged.
metadata:
  short-description: Author a reviewable Playwright mobile test with DeviceDeck
---

# Authoring Playwright tests with DeviceDeck

DeviceDeck mirrors a simulator/emulator's native UI as **real DOM** over a live video of the
screen, so Playwright drives it exactly like a web page — same API, no Appium. What is
*different* is that you are driving a **device running a native app**, and this skill is that
difference: durable accessibility-id selectors, real-HID typing, native gestures, and app
lifecycle a web test would not know.

**Prerequisites**
- `devicedeck` is running (default `http://127.0.0.1:8787`). The device page is
  `/device/{udid}?app={bundleId}` — use `booted` for the udid when one simulator is up.
- **If no device is booted, boot one yourself** — do not send the user to the console. With
  DeviceDeck's MCP tools: `list_devices`, then `boot_device` on a simulator (or emulator) that
  fits the app, then `launch_app` with the bundle id (a build registered with `devicedeck --app`
  is installed first). `device_page_url` gives the page to open. Only without those tools, ask the
  user to open the console at the base URL and pick a device.
- You drive with **Playwright MCP** (`@playwright/mcp`) — the browser tool you already use for
  the web. (Cypress and Puppeteer drive the same DOM too — see [`examples/`](../../examples/) —
  but Playwright is the natural pairing, since it is also what drives.)

## Explore the device with Playwright MCP

The tools are your normal Playwright browser tools; the **awareness** below is the device layer.

1. **Navigate** to `http://127.0.0.1:8787/device/booted?app={bundleId}`.
2. **Snapshot** — the native screen comes back as a DOM tree; every element carries its
   `data-testid` (the app's accessibility id) and an ARIA role/name.
3. **Act by selector** — `getByTestId` / `getByRole`, never a coordinate.
4. **Confirm a typed value landed before you submit.** Keys reach the device as real HID
   presses a moment after the DOM shows them; the submit button usually enables only once the
   app has accepted the value. Submitting too early acts on a half-typed form — unlike a
   normal web input.
5. **Device-only controls** no DOM element exposes live on `window.devicedeck`, via
   `page.evaluate`:
   - `devicedeck.gesture('home' | 'appSwitcher' | 'notificationCenter' | 'lockScreen')`
   - `devicedeck.swipe(x1, y1, x2, y2, durationMs)` — normalized 0–1 coordinates
   - `devicedeck.button(name)` · `devicedeck.key(usage, modifiers)` — hardware keys
   - `devicedeck.screenshot()` — the device screen alone as a PNG data URL (no browser
     chrome, no mirror overlay), for reasoning over pixels; needs video streaming
   - raw `devicedeck.tap(x, y)` — a last resort; prefer a `data-testid` click
   The device page `<title>` advertises the gesture set to any agent that reads it.

## Emit the Playwright test

Write it like any web spec, with the same `data-testid` selectors — follow
[`examples/playwright`](../../examples/playwright) as the canonical template. The idioms that
matter here:

- **Project:** if the folder has no Playwright setup, create one so `npx playwright test` runs:
  `package.json` with `@playwright/test` as a dev dependency, and a `playwright.config.ts`.
- **Config:** `baseURL: 'http://127.0.0.1:8787'`, `workers: 1`, `fullyParallel: false` — a
  device serves one driver at a time.
- **Launch fresh in a fixture:** `request.post('/api/devices/{udid}/app/launch', { data: { app } })`
  — it blocks until the app is taking input, so the first action lands. It clears the app's data
  first; post to `/app/launch?reset=no` to resume where the app was left instead.
- **Selectors:** `page.getByTestId('login-button')` / `getByRole`; Playwright auto-waits.
- **Typing:** `page.keyboard.type(text, { delay: 150 })` — real HID pacing.
- **Wait for the echo before submitting:** `page.waitForFunction` on the field's
  `data-dd-device-value` (a secure field reports bullets — check its *length*).
- **First render** waits on the tree-engine warm-up — give the first selector ~30s.
- **Gestures:** `await page.evaluate(() => devicedeck.gesture('home'))`.

## Rules that keep a test durable

- **Selectors, never coordinates.** Drive and assert by `data-testid`; a test that used pixels
  reviews as noise and breaks on the first layout change.
- **Wait for the device to echo a typed value before submitting.** Keys land a moment after the
  DOM has them; a submit fired too early acts on a half-typed form.
- **Reset between independent tests.** A native app stays logged in across relaunch; a fresh
  launch (the default) is the clean first-run slate.
- **One driver per device.** Run a single worker, and close any console tab left open on the
  device or it will refuse the run.

## Managing devices

With DeviceDeck's MCP tools (the Claude plugin, the Gemini extension, or `devicedeck mcp` added by
hand) the agent picks, boots, installs and launches itself: `list_devices`, `boot_device`, `install_app`
(a `.app` for a Simulator or a `.apk` for an emulator — not a device `.ipa`), and `launch_app`
(which also takes an `appFile` to install-then-launch in one call); `device_page_url` returns the
page to hand to Playwright MCP. Without them, a device the user booted plus Playwright MCP is all you
need.

**If the app is not installed** (a launch fails with *"is it installed?"*) **or you were not given
its file, ask the user** for the `.app`/`.apk` path or the bundle id — do not guess a bundle id or
fabricate a path.
