# DeviceDeck

**Your simulators and emulators in a browser — and the session you drive by hand becomes a test
your existing framework can run.**

A single Go binary. It streams an iOS Simulator or Android emulator to a browser, mirrors the
app's native UI tree as **real DOM**, and records what you do into a replayable Maestro flow.

The mirror is the point. Because the device is an ordinary web page, the tools you already own
drive it with no mobile-specific code and no Appium:

```ts
await page.goto(`/device/${udid}?app=dev.devicelab.testhive`);
await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
await page.getByRole('textbox', { name: 'Password' }).fill('robustest');
await page.getByRole('button', { name: 'Sign In' }).click();
await expect(page.getByText('Hello, devicelab!')).toBeVisible();
```

That is a real iOS Simulator being driven by stock Playwright.

## Status

Working, not yet released. Both platforms drive end to end: **Playwright, Cypress and Puppeteer
each log into TestHive through the same DOM** — it is an ordinary web page, so any browser driver
works with no mobile-specific code. Typing is checked against the device's own read-back, so a
keystroke that does not land is retyped rather than lost, on iOS *and* on Android's masked
password fields. A captured flow replays unchanged under a stock `maestro-runner` — verified at
50/50 on Android with assertions, and a deliberate negative control confirming the gate can fail.

What that sentence does not cover: it has only ever run on the machine it was built on. Expect
first-contact problems. See **Known limits** below.

## Requirements

- macOS (the video and input sidecars talk to CoreSimulator)
- Xcode with at least one iOS Simulator runtime — **iOS 26.2 or newer is strongly recommended**;
  on 18.6 the simulator's render server crashes under repeated capture
- For Android: the Android SDK with `adb` and `emulator` on `PATH`

## Install

Download a release and run it — the archive holds the binary and its two sidecars, which it
expects to find beside itself:

```bash
tar xzf devicedeck-<version>-darwin-arm64.tar.gz
cd devicedeck-<version>-darwin-arm64
./devicedeck serve
```

From source:

```bash
make sidecar      # builds the Swift sidecars (macOS, Xcode toolchain)
make build
./devicedeck serve
```

Then open <http://127.0.0.1:8787>.

## Using it

**The console** lists every simulator and emulator on the machine, running or not. Click one to
boot it and start streaming. Drive it with your mouse and keyboard; **Inspect** overlays the
native tree; **Record** captures the session.

**Flow Capture** turns what you did into a Maestro flow with durable selectors — accessibility
identifiers, never coordinates — so it replays on real hardware, not just here:

```yaml
appId: com.testhiveapp
---
- launchApp
- assertVisible:
    id: "login-button"
- tapOn:
    id: "username-input"
- inputText: "devicelab"
- tapOn:
    id: "login-button"
- assertVisible:
    id: "product-item-Maestro"
```

Arm **Assert** and the next tap records an assertion instead of tapping. A point that resolves
to nothing addressable is refused rather than recorded as a coordinate — "there are pixels here"
is not worth putting in a flow.

**Only the screen you are on is mirrored.** iOS keeps a screen in the hierarchy after the app
navigates away from it, so the tree reports views nobody can see — a login form, credentials
still in its fields, present on every screen that follows. Those are culled, because a selector
that resolves to an invisible screen fails silently here and again on real hardware.

**Text fields are real `<input>` elements**, so `fill()`, `inputValue()` and `toHaveValue()` work
as they would on any page — and so does the `browser_type` an AI agent reaches for first. A
field's contents live in its value, not its text, which is the one place the mirror departs from
"every native node is a div".

**Your own tests** point at `/device/{udid}?app={bundleId}` and use ordinary selectors.
`examples/` has runnable Playwright, Cypress and Puppeteer projects, and `examples/captured/`
holds flows recorded through the console — including one from a Flutter app and one from React
Navigation. Note that **a device serves one driver at a time**; run with a single worker.

**Agents drive it through an MCP server.** `devicedeck mcp` speaks the Model Context Protocol on
stdin/stdout — a thin adapter over the same API — so an agent (Claude, Cursor, any MCP client)
lists, boots, and launches devices, inspects them (`ui_tree`, `screenshot`), and acts by durable
selector (`tap`, `assert_visible`), then opens the device page to drive typing with its own browser
tools. A Claude Code plugin bundles the server with authoring, triage, and flow-capture skills —
install it with `claude plugin marketplace add devicelab-dev/DeviceDeck`. See
[`examples/mcp/`](examples/mcp/).

## Scope

Simulators and emulators only. Real hardware is deliberately out of scope; that is
[devicelab.dev](https://devicelab.dev)'s job, and the boundary is what keeps the two from
competing. A flow captured here is meant to run there unchanged — that is the point of insisting
on durable selectors.

## Known limits

- **Android video is ~18 fps** against iOS's ~25, and costs roughly ten times the bandwidth: the
  emulator's gRPC screenshot stream offers no video codec, so every frame is a full PNG.
  Emulators DeviceDeck boots run headless, because macOS throttles an occluded window and the
  emulator's window is occluded exactly when you are watching the browser.
- **A freshly launched app swallows touches for about a second** after its screen is already in
  the accessibility tree, reporting itself hittable and stable throughout — so there is nothing
  to wait on but the effect. `POST /app/launch` waits this window out for you: it returns when the
  app is actually taking input, so a test that launches through it can act at once. A raw
  terminate+launch outside the endpoint cannot, and must prove the app is taking input first.
- **Two-finger gestures are dropped on Android.** They work on iOS; there is no mapping for them
  in the Android driver, and they are discarded rather than guessed at.
- **A device serves one driver at a time**, and a second claim is refused rather than shared.
  That includes the console: a browser tab left open on a device will refuse your test run,
  and the refused page says so on screen. A driver that dies without closing its socket is
  detected by ping within about half a minute, and the device is released.
- **A raw relaunch does not reset app state** — a native app stays logged in across
  terminate+launch, the surprise that makes web-style tests flaky against it. `POST /app/launch`
  wipes the app's data first *by default*, starting at a first-run screen the way a new automation
  session expects; pass `?reset=no` to resume where the last session left off.

## Licence

Apache License 2.0 — see `LICENSE`.

## Built on

[maestro-runner](https://github.com/devicelab-dev/maestro-runner) for device drivers and flow
replay, both Apache-2.0. The Swift sidecars derive from
[baguette](https://github.com/tddworks/baguette) (Apache-2.0) and
[tapflow](https://github.com/jo-duchan/tapflow) (MIT); `ATTRIBUTION.md` records what was reused
and where it lives.

Built by [DeviceLab.dev](https://devicelab.dev)
