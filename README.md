<div align="center">

# DeviceDeck

**Your simulators and emulators become a webpage — drive them with the test tools you already
own, and turn a session you did by hand into a test that runs unchanged on real hardware.**

<sub>iOS&nbsp;+&nbsp;Android&nbsp;·&nbsp;native UI mirrored as **real DOM**&nbsp;·&nbsp;Flow&nbsp;Capture&nbsp;·&nbsp;MCP&nbsp;server&nbsp;for&nbsp;agents&nbsp;·&nbsp;Playwright&nbsp;/&nbsp;Cypress&nbsp;/&nbsp;Puppeteer&nbsp;with&nbsp;zero&nbsp;adapter</sub>

![License](https://img.shields.io/badge/license-Apache_2.0-blue.svg)
![Platform](https://img.shields.io/badge/platform-macOS-lightgrey?logo=apple)
![iOS + Android](https://img.shields.io/badge/devices-iOS_%2B_Android-success)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Single binary](https://img.shields.io/badge/ships-single_binary-brightgreen)

[Quick start](#quick-start) · [Examples](examples/) · [Drive with an agent](examples/mcp/) · [How it works](#the-mirror-is-the-point) · [Known limits](#known-limits)

</div>

---

## The mirror is the point

A single Go binary. It streams an iOS Simulator or Android emulator to a browser, mirrors the
app's native UI tree as **real DOM**, and records what you do into a replayable Maestro flow.

Because the device is an ordinary web page, the tools you already own drive it with no
mobile-specific code, no Appium, and no coordinates:

```ts
await page.goto(`/device/${udid}?app=dev.devicelab.testhive`);
await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
await page.getByRole('textbox', { name: 'Password' }).fill('robustest');
await page.getByRole('button', { name: 'Sign In' }).click();
await expect(page.getByText('Hello, devicelab!')).toBeVisible();
```

That is a real iOS Simulator being driven by stock Playwright. The same page, the same selectors,
drive an Android emulator — and an AI agent drives it through an MCP server.

## What you get

- **A device that is a webpage.** The native accessibility tree is mirrored as real DOM —
  `data-testid` from identifiers, ARIA roles from element types — so `getByTestId`, `getByRole`,
  `click`, and `fill` all just work. No driver, no DSL, no coordinates.
- **The tools you already know.** Playwright, Cypress, and Puppeteer each drive both platforms
  with zero adapter; any browser driver does. See runnable [`examples/`](examples/).
- **Flow Capture.** Record a session by hand and it becomes a Maestro flow with **durable
  selectors** that replays unchanged on real hardware — the funnel from "I did this once" to
  "this is a test."
- **Agents, first-class.** `devicedeck mcp` is a Model Context Protocol server, and a
  [Claude Code plugin](examples/mcp/) ships with it — an agent lists, boots, launches, inspects,
  taps, and screenshots devices through the one core.
- **Typing that actually lands.** Every keystroke is verified against the device's own read-back
  and retyped if it drifted, so a loaded machine does not silently drop a character.
- **One binary.** No relay, no agent split, no per-tool fork — the same core behind every surface.
- **One host, any client.** The device is a webpage, so one Mac runs the whole simulator and
  emulator farm and serves it on your network (`serve --addr 0.0.0.0:8787`); teammates on Linux,
  Windows, or another Mac drive them from their own browser, tests, or agent — no macOS on the
  client side. A device serves one driver at a time, so it is a farm of *N* devices for *N* people.

## Why not Appium

Every mobile testing framework rebuilds the same four primitives — a driver, a selector language,
a way to send input, a way to read the tree. DeviceDeck doesn't ship any of them. It makes the
device a **web page**, so a web driver you already have *is* the driver, its selectors *are* the
selectors, and its assertions *are* the assertions. The one thing it insists on is **durable
selectors over coordinates**, because that is what lets a flow captured on a simulator run
unchanged on a real device — which is the whole point.

## Status

Working, not yet released. Both platforms drive end to end: **Playwright, Cypress and Puppeteer
each log into TestHive through the same DOM** — it is an ordinary web page, so any browser driver
works with no mobile-specific code. Typing is checked against the device's own read-back, so a
keystroke that does not land is retyped rather than lost, on iOS *and* on Android's masked
password fields. A captured flow replays unchanged under a stock `maestro-runner` — verified at
50/50 on Android with assertions, and a deliberate negative control confirming the gate can fail.

What that sentence does not cover: it has only ever run on the machine it was built on. Expect
first-contact problems. See [**Known limits**](#known-limits) below.

## Requirements

- **macOS to *host*** — iOS Simulators, `simctl`/CoreSimulator, and the Swift sidecars are
  macOS-only, so the machine that runs the devices is a Mac. **Clients are any OS:** the surface is
  a web page, so tests and agents drive it from Linux, Windows, or another Mac over the network
- **Xcode** with at least one iOS Simulator runtime — **iOS 26.2 or newer is strongly
  recommended**; on 18.6 the simulator's render server crashes under repeated capture
- **For Android:** the Android SDK with `adb` and `emulator` on `PATH`

## Quick start

Download a release and run it — the archive holds the binary and its two sidecars, which it
expects to find beside itself:

```bash
tar xzf devicedeck-<version>-darwin-arm64.tar.gz
cd devicedeck-<version>-darwin-arm64
./devicedeck serve
```

Then open **<http://127.0.0.1:8787>**, pick a device, and it boots and starts streaming.

<details>
<summary><strong>Build from source</strong></summary>

```bash
make sidecar      # builds the Swift sidecars (macOS, Xcode toolchain)
make build
./devicedeck serve
```

</details>

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
[`examples/`](examples/) has runnable Playwright, Cypress and Puppeteer projects — each with a
login and a checkout journey — and [`examples/captured/`](examples/captured/) holds flows recorded
through the console, including one from a Flutter app and one from React Navigation. Note that
**a device serves one driver at a time**; run with a single worker.

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
- **The server is unauthenticated.** It binds to `127.0.0.1` by default; exposing it with
  `--addr` to share devices with a team is fine on a trusted network, but there is no access
  control yet — do not hang it on the open internet as-is. A LAN/tunnel-with-auth story is planned,
  not built.

## Licence

Apache License 2.0 — see [`LICENSE`](LICENSE).

## Built on

[maestro-runner](https://github.com/devicelab-dev/maestro-runner) for device drivers and flow
replay, both Apache-2.0. The Swift sidecars derive from
[baguette](https://github.com/tddworks/baguette) (Apache-2.0) and
[tapflow](https://github.com/jo-duchan/tapflow) (MIT); [`ATTRIBUTION.md`](ATTRIBUTION.md) records
what was reused and where it lives.

Built by [**DeviceLab.dev**](https://devicelab.dev)
