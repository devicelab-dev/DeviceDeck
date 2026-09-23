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
difference: durable accessibility-id selectors, actions that wait for the device, native
gestures, and app lifecycle a web test would not know.

**Prerequisites**
- `devicedeck` is running (default `http://127.0.0.1:8787`). The device page is
  `/device/{udid}?app={bundleId}`, where the udid is a simulator's UDID or an emulator's serial
  (`emulator-5554`). Use the explicit id from `list_devices`; `booted` is only a shortcut for
  when exactly one simulator and no emulator is up.
- **Find the app with `list_apps`** when the user says "my app" rather than a bundle id: it
  lists the builds registered with `devicedeck --app` (name, bundle id, platform, minimum OS).
  Pick the one that matches; ask only if several do.
- **If no device is booted, boot one yourself** — do not send the user to the console. With
  DeviceDeck's MCP tools: `list_devices`, then `boot_device` on a simulator (or emulator) of the
  app's platform, then `launch_app` with the bundle id (a registered build is installed first). `device_page_url` gives the page to open. Without the MCP tools, the same steps are HTTP
  calls: `GET /api/devices`, `GET /api/apps`, `POST /api/devices/{id}/boot`,
  `POST /api/devices/{udid}/app/launch` with `{"app": "<bundle id>"}`. Only if you can do none of
  these, ask the user to open the console at the base URL and pick a device.
- **Boot returns before the device is up.** Poll `list_devices` (or `GET /api/devices`) every few
  seconds until it is listed booted — typically 10–30s, longer on a cold machine. A powered-off
  emulator is listed as `avd:{name}`; boot it by that id, and it comes back under a serial such
  as `emulator-5554` — use the serial from then on.
- **Launch blocks until the app takes input**, so the first action after it lands. Right after a
  boot, an emulator can be listed booted while Android is still starting: a launch then fails
  with a raw `adb … monkey … exit status` error. Wait a few seconds and launch again; only
  *"is it installed?"* means the build is missing. Expect a first launch to take ~20s on iOS
  (the tree engine starts) and a few seconds after that.
- You drive with **Playwright MCP** (`@playwright/mcp`) — the browser tool you already use for
  the web — and emit a Playwright spec.

## Explore the device with Playwright MCP

The tools are your normal Playwright browser tools; the **awareness** below is the device layer.

1. **Navigate** to `http://127.0.0.1:8787/device/{udid}?app={bundleId}`, after `launch_app`.
2. **Snapshot** — the native screen comes back as a DOM tree; every element carries its
   `data-testid` (the app's accessibility id) and an ARIA role/name.
3. **Act by selector** — `getByTestId` / `getByRole`, never a coordinate.
4. **Actions wait for the device.** `browser_click` and `browser_type` (which emits `fill()`)
   return once the device has acted and the screen has settled, so the next snapshot shows the
   result — no waiting needed. Two exceptions to know: an app that updates on a timer (a
   debounced search) can change after the snapshot — assert with `expect`, which retries; and a
   button the app has disabled can show only as its text, with no ref and no test id, until it is
   enabled — fill the form, then snapshot again to find it.
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
  `package.json` with `@playwright/test` as a dev dependency, and a `playwright.config.ts`; then
  `npm install` and `npx playwright install chromium`. If it already has a Playwright setup for a
  website, keep it: add the device tests as their own entry in `projects` (with the device
  `baseURL`) rather than rewriting the config.
- **Config:** `baseURL: 'http://127.0.0.1:8787'`, `workers: 1`, `fullyParallel: false` — a
  device serves one driver at a time.
- **Launch fresh in a fixture:** `request.post('/api/devices/{udid}/app/launch', { data: { app } })`
  — it blocks until the app is taking input, so the first action lands. It clears the app's data
  first; post to `/app/launch?reset=no` to resume where the app was left instead.
- **The device in the spec:** never hard-code the UDID or serial you authored on — it names a
  simulator on this Mac only. Read `process.env.DEVICEDECK_UDID`; when it is unset, have the
  fixture pick a booted device of the app's platform from `GET /api/devices` (`{"devices": [{udid,
  name, os, booted}]}`, where `os` starts with `iOS` or `android`). The spec then runs unchanged
  on a teammate's Mac and in CI.
- **Credentials from the environment:** `process.env.TEST_USER` / `TEST_PASSWORD` (or the
  names the project already uses) — never write a real password into the spec. If you do not
  know the login, ask the user; do not invent one.
- **Selectors:** `page.getByTestId('login-button')` / `getByRole`; Playwright auto-waits.
- **Typing:** `locator.fill(text)` — the same code Playwright MCP emits. It returns once the
  device holds the value, so there is nothing to wait for before submitting. Avoid
  `keyboard.type`: key by key it raises Android's soft keyboard, and the tap after it can be
  spent closing the keyboard instead of pressing the button.
- **First render** waits on the tree-engine warm-up — give the first selector ~30s.
- **Gestures:** `await page.evaluate(() => devicedeck.gesture('home'))`.
- **Run it before you hand back:** `npx playwright test`. If it fails, diagnose with
  `devicedeck-triage`, fix, and run again until it passes; then report what it covers and that it
  passed.

## Rules that keep a test durable

- **Selectors, never coordinates.** Drive and assert by `data-testid`; a test that used pixels
  reviews as noise and breaks on the first layout change.
- **No fixed waits.** `click()` and `fill()` return once the device has acted; a
  `waitForTimeout` only hides a real problem. Assert with `expect`, which retries.
- **Reset between independent tests.** A native app stays logged in across relaunch; a fresh
  launch (the default) is the clean first-run slate.
- **One driver per device.** Run a single worker, and close any console tab left open on the
  device or it will refuse the run.

## Managing devices

With DeviceDeck's MCP tools (the Claude plugin, the Gemini extension, or `devicedeck mcp` added by
hand) the agent picks, boots, installs and launches itself: `list_devices`, `list_apps`, `boot_device`, `install_app`
(a `.app` for a Simulator or a `.apk` for an emulator — not a device `.ipa`), and `launch_app`
(which also takes an `appFile` to install-then-launch in one call); `device_page_url` returns the
page to hand to Playwright MCP. Without them, a device the user booted plus Playwright MCP is all you
need.

**If the app is not installed** (a launch fails with *"is it installed?"*) **or you were not given
its file, ask the user** for the `.app`/`.apk` path or the bundle id — do not guess a bundle id or
fabricate a path.

**If the device is stuck, restart it.** Relaunch the app first (`launch_app`). If the page stays empty
or the app sits on its splash screen after that, the device itself is wedged: shut it down and boot
it again, then launch the app. There is no MCP tool for the shutdown; use the HTTP API, which works
from Playwright MCP too — the device page is served by the same server, so `browser_evaluate` can
call it:

```js
// 1. Shut the device down (ends its session; whoever drives it is told so).
await fetch('/api/devices/{udid}/shutdown', { method: 'POST' });
// 2. Boot it again. A simulator boots by its UDID. An emulator loses its serial when it
//    powers off, so boot it by its AVD — the "avd" field of GET /api/devices: `avd:{name}`.
await fetch('/api/devices/{udid-or-avd:name}/boot', { method: 'POST' });
// 3. Poll GET /api/devices until the device is listed booted (an emulator reappears with
//    a serial, which may differ from before), then launch the app on it.
```

Restart only a device you were given or booted yourself — never one the user is working on.

**When you are done,** say which device you booted, if you booted one, and offer to shut it down
(`POST /api/devices/{udid}/shutdown`): a simulator or emulator left running holds several GB of
memory. Leave a device the user booted as it is.
