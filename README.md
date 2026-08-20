# DeviceDeck

**Your simulators and emulators in a browser — and the session you drive by hand becomes a test
your existing framework can run.**

A single Go binary. It streams an iOS Simulator or Android emulator to a browser, mirrors the
app's native UI tree as **real DOM**, and records what you do into a replayable Maestro flow.

The mirror is the point. Because the device is an ordinary web page, the tools you already own
drive it with no mobile-specific code and no Appium:

```ts
await page.goto(`/device/${udid}?app=dev.devicelab.testhive`);
await page.getByRole('textbox', { name: 'Username' }).click();
await page.keyboard.type('devicelab');
await page.getByRole('button', { name: 'Sign In' }).click();
await expect(page.getByText('Hello, devicelab!')).toBeVisible();
```

That is a real iOS Simulator being driven by stock Playwright.

## Status

Working, not yet released. Both platforms drive end to end, and a captured flow replays
unchanged under a stock `maestro-runner` — verified at 50/50 on Android with assertions, and a
deliberate negative control confirming the gate can fail.

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

**Your own tests** point at `/device/{udid}?app={bundleId}` and use ordinary selectors.
`examples/` has runnable Playwright, Cypress and Puppeteer projects, and `examples/captured/`
holds flows recorded through the console — including one from a Flutter app and one from React
Navigation. Note that **a device serves one driver at a time**; run with a single worker.

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
- **A freshly launched app accepts touches about a second after it appears** in the accessibility
  tree, and reports itself hittable and stable throughout — so there is nothing to wait on but
  the effect. Tests should prove the app is taking input before relying on it.
- **Two-finger gestures are dropped on Android.** They work on iOS; there is no mapping for them
  in the Android driver, and they are discarded rather than guessed at.
- **A device serves one driver at a time**, and a second claim is refused rather than shared.
  That includes the console: a browser tab left open on a device will refuse your test run,
  and only the refused side is told why. There is no keepalive yet, so a client that dies
  without closing its socket holds the device until the server restarts.
- Relaunching an app does not reset its state.

## Licence

Apache License 2.0 — see `LICENSE`.

## Built on

[maestro-runner](https://github.com/devicelab-dev/maestro-runner) for device drivers and flow
replay, both Apache-2.0. The Swift sidecars derive from
[baguette](https://github.com/tddworks/baguette) (Apache-2.0) and
[tapflow](https://github.com/jo-duchan/tapflow) (MIT); `ATTRIBUTION.md` records what was reused
and where it lives.

Built by [DeviceLab.dev](https://devicelab.dev)
