# Playwright against an iOS simulator or Android emulator

The device page (`/device/{udid}?app={bundleId}`) shows the simulator or emulator
with the app's native UI mirrored as real DOM: `data-testid` from accessibility
identifiers (React Native's `testID`, Android's `resource-id`), ARIA roles from
element types, text from labels. Playwright drives it like any web page — same
API, same runner, same HTML report. No Appium, no WebDriverAgent.

## Run the examples

```bash
# terminal 1: DeviceDeck, with the app registered so it can be installed and launched
devicedeck --app path/to/TestHive.app --app path/to/app-release.apk

# terminal 2
npm install
npx playwright install chromium

# iOS — a booted simulator
DEVICEDECK_UDID=<simulator udid> npx playwright test tests/agent-written/testhive-ios.spec.ts

# Android — a booted emulator
DEVICEDECK_ANDROID_SERIAL=emulator-5554 npx playwright test tests/agent-written/testhive-android.spec.ts
```

Each test launches the app fresh (its data is cleared), so every test starts
logged out and independent.

## Start here: tests an agent wrote

[`tests/agent-written/`](tests/agent-written) holds five tests per platform —
wrong password, login, search, category filter, cart and checkout — written by
Claude with Playwright MCP. It was told only "test my app" and the login; it
booted the device, explored the app, and wrote the spec from the code
Playwright MCP emitted. Both files passed twice, as written, on DeviceDeck 1.0.1.

To do the same for your app, give your agent Playwright MCP and DeviceDeck's
plugin, and ask: *"Write Playwright tests that cover the main journeys of my
app."*

## What is different from a web test

Very little, which is the point. The idioms that matter:

- **Launch fresh in `beforeEach`:**
  `request.post('/api/devices/{udid}/app/launch', { data: { app } })` returns once
  the app is taking input. Add `?reset=no` to keep the app's data instead.
- **Type with `fill()`.** It returns once the device holds the value, so there
  is nothing to wait for before submitting. Avoid `keyboard.type`: key by key it
  raises Android's soft keyboard, and the next tap can be spent closing it.
- **Actions wait for the device.** `click()` and `fill()` return once the device
  has acted and the screen has settled — no `waitForTimeout`. Assert with
  `expect`, which retries: an app that updates on a timer (a debounced search)
  can change a moment later.
- **One worker.** A device serves one driver at a time; `playwright.config.ts`
  sets `workers: 1`.
- **Gestures** DOM events cannot express are available from scripts:
  `page.evaluate(() => devicedeck.swipe(0.5, 0.8, 0.5, 0.2))`,
  `devicedeck.gesture('home')`.

## Android notes

Run emulators with `hw.keyboard = yes` (Android Studio's default): with a
hardware keyboard the soft keyboard stays closed, so the layout doesn't reshape
mid-flow.

## What else is in this folder

- [`tests/app/`](tests/app) — earlier generated specs for the same app.
- [`tests/recorded/`](tests/recorded) — a spec recorded in DeviceDeck's console.
- [`tests/platform/`](tests/platform) — DeviceDeck's own checks of the device
  page (mirror fixtures, scrolling, overlays, frame rate). Not examples of how
  to write an app test.
- [`demo/`](demo) — the script behind the README's demo recording.
