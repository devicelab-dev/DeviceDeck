<div align="center">

# DeviceDeck

### Automate your iOS and Android app like a web app.

Your Playwright tests and AI agents drive it with the browser tools they already use.
No Appium, no new tool.

<img src="docs/demo-login.gif" width="820" alt="A stock Playwright test on the left runs line by line — page.goto() to a Simulator on a Mac at 10.0.4.21, getByRole().fill() for username and password, click Sign In, expect the logged-in screen — while a browser tab on the right shows the iOS Simulator reacting live and the runner goes green.">

Stock Playwright driving a real iOS Simulator from another machine — the same DOM an AI agent drives too.

<p>
<a href="docs/testing.md"><img alt="Playwright" src="https://img.shields.io/badge/Playwright-2EAD33"></a>
<a href="examples/cypress"><img alt="Cypress" src="https://img.shields.io/badge/Cypress-17202C?logo=cypress&logoColor=white"></a>
<a href="examples/puppeteer"><img alt="Puppeteer" src="https://img.shields.io/badge/Puppeteer-40B5A4?logo=puppeteer&logoColor=white"></a>
<a href="docs/agents.md"><img alt="Playwright MCP" src="https://img.shields.io/badge/Playwright_MCP-2EAD33?logo=modelcontextprotocol&logoColor=white"></a>
<a href="docs/agents.md"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-D97757?logo=claude&logoColor=white"></a>
<a href="#record-a-flow"><img alt="Maestro flows" src="https://img.shields.io/badge/Maestro_flows-4f8cff"></a>
<a href="https://github.com/devicelab-dev/maestro-runner"><img alt="maestro-runner" src="https://img.shields.io/badge/maestro--runner-4f8cff"></a>
</p>
<p>
<img alt="iOS Simulator" src="https://img.shields.io/badge/iOS_Simulator-000000?logo=apple&logoColor=white">
<img alt="Android Emulator" src="https://img.shields.io/badge/Android_Emulator-3DDC84?logo=android&logoColor=white">
<img alt="macOS host" src="https://img.shields.io/badge/host-macOS-lightgrey?logo=apple">
<img alt="Single binary" src="https://img.shields.io/badge/ships-single_binary-brightgreen">
</p>
<p>
<a href="LICENSE"><img alt="License: Apache 2.0" src="https://img.shields.io/badge/license-Apache_2.0-blue"></a>
<img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
<a href="https://devicelab.dev"><img alt="by DeviceLab.dev" src="https://img.shields.io/badge/by-DeviceLab.dev-4f8cff"></a>
</p>

[What it does](#what-it-does) · [Quick start](#quick-start) · [Using it](#using-it) · [Docs](docs/) · [Known limits](#known-limits)

</div>

---

## What it does

**Share your simulators.** Run `devicedeck` on a Mac and everyone on your network can drive its
simulators and emulators from their own browser — tap, type, and **Inspect** the native UI — with
no Xcode, no Android Studio, and no device of their own.

**Automate them with the tools you already use.** The app's native UI is served as an ordinary web
page, so Playwright, Cypress and Puppeteer tests drive it by selector, and your AI agent drives it
through [Playwright MCP](https://github.com/microsoft/playwright-mcp) — the same browser tool it
uses for the web. Nothing mobile-specific to learn.

**Record a flow.** Use the app by hand in the console and DeviceDeck writes it down as a
[Maestro](https://maestro.dev) flow, with durable selectors, ready to review and replay — unchanged
on real devices at [devicelab.dev](https://devicelab.dev).

One binary behind all three — nothing to fork, no Xcode project to open.

## How it works

The native UI tree is mirrored as **real DOM** — accessibility ids become `data-testid`, element
types become ARIA roles — so stock selectors drive it, whether from an agent or a test:

```ts
await page.goto(`/device/${udid}?app=dev.devicelab.testhive`);
await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
await page.getByRole('button', { name: 'Sign In' }).click();
await expect(page.getByText('Hello, devicelab!')).toBeVisible();
```

A single Go binary streams the simulator or emulator to the browser and serves that DOM — the same
page and selectors drive iOS and Android alike.

## Status

Early release. Both platforms drive end to end: **Playwright, Cypress and Puppeteer each log into and
check out of TestHive through the same DOM**, and the console drives any device by hand. It is an
ordinary web page, so any browser driver works with no mobile-specific code. Typing is checked
against the device's own read-back, so a keystroke that does not land is retyped rather than lost —
on iOS *and* on Android's masked password fields.

It has run on a handful of Macs so far, so expect some first-contact problems — please
[open an issue](https://github.com/devicelab-dev/DeviceDeck/issues) with the log folder it prints.
See [**Known limits**](#known-limits).

## Requirements

- **A Mac to host.** iOS Simulators, `simctl`/CoreSimulator and the Swift sidecars are
  macOS-only, so the machine that runs the devices is a Mac. **Clients can be any OS:** the surface
  is a web page, so people, tests and agents drive it from Linux, Windows or another Mac.
- **Xcode** with at least one iOS Simulator runtime — **iOS 26.2 or newer is strongly
  recommended**; on 18.6 the simulator's render server crashes under repeated capture.
- **For Android:** the Android SDK, with `adb` and `emulator` on `PATH`, and at least one virtual
  device.

`devicedeck doctor` checks all of this and says how to fix anything missing.

## Quick start

**Install** with the script — it puts DeviceDeck in `~/.devicedeck` and adds its `bin` folder to
your `PATH`. No sudo, and nothing else to install: the Android driver ships inside the binary.

```bash
curl -fsSL https://open.devicelab.dev/install/devicedeck | bash
# a specific version:
curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.0
```

Or download an archive from [Releases](https://github.com/devicelab-dev/DeviceDeck/releases) and run
it in place (the two sidecars sit beside the binary in `bin/`):

```bash
tar xzf devicedeck-<version>-darwin-arm64.tar.gz
./devicedeck-<version>-darwin-arm64/bin/devicedeck
```

Or build from source (needs the Xcode toolchain for the Swift sidecars): `make sidecar && make build`.

**Run it:**

```bash
devicedeck                                                    # the console: http://127.0.0.1:8787
devicedeck --app build/MyApp.app --app build/app-release.apk  # …with your app builds
```

It prints the console link (and the network address teammates use), your registered apps, how to
connect Claude Code, and any missing tools with how to fix them. Open the console, pick a device,
and it boots, starts streaming, and launches your app.

With `--app`, a build is installed the first time it is launched on a device — from the console, a
test or Claude. Pass `.app` simulator builds and `.apk` files, or a folder of them.

Everything DeviceDeck writes lives in `~/.devicedeck` (set `DEVICEDECK_HOME` to move it). To
uninstall, delete that folder and the `# DeviceDeck` line from your shell profile.

## Using it

### With your tests

Point Playwright, Cypress or Puppeteer at `/device/{udid}?app={bundleId}` — accessibility ids are
`data-testid`, roles and labels are ARIA, so you drive it with ordinary selectors.
[`examples/`](examples/) has runnable login and checkout projects for all three. **Guide:**
[docs/testing.md](docs/testing.md).

**One device, one worker.** A device takes one driver at a time: two clients tapping at once would
interleave into nonsense, so a second one is refused and told who holds the device. Test runners go
parallel by default, so set one worker per device (`workers: 1` in Playwright) — and to run in
parallel, boot more devices and give each worker its own. In the console, tabs hand the device to
the tab you are using, and **Take over** disconnects whoever holds it.

### With an AI agent

Add the browser tool your agent already has, then DeviceDeck's skills:

```bash
claude mcp add playwright npx @playwright/mcp@latest
claude plugin marketplace add devicelab-dev/DeviceDeck
claude plugin install devicedeck@devicedeck-marketplace
```

The agent drives the device the way it drives the web: it **snapshots the page**, reasons over the
tree, and acts by ref — no bespoke tool, no coordinates, no vision model. Here it picks the right
**Add** among five identical ones and adds *Maestro* to the cart:

<div align="center">
<img src="docs/demo-agent.gif" width="820" alt="An AI agent using Playwright MCP: browser_snapshot returns the product screen as an accessibility tree with five identical Add buttons, a reasoning step picks the one after Maestro (add-to-cart-2, ref e24), browser_click adds it, and opening the cart confirms Maestro was added — not Appium.">
</div>

That disambiguation is an [included spec](examples/playwright/tests/platform/mcp-agent.spec.ts) that
passes end to end. Using Codex, Cursor, Gemini CLI or VS Code? Each has a two- or three-command
setup in [docs/agents.md](docs/agents.md#set-up-your-agent).

### By hand — the console

The console lists every simulator and emulator on the Mac, and every build you passed with `--app`.
Pick a device to boot and stream it, or **Launch** an app straight onto a running one. Drive it with
your mouse and keyboard; **Inspect** overlays the native tree and shows each element's id, role and
text — the selectors a test or an agent will use. **End session** shuts the device down when you are
done.

Teammates open the network address `devicedeck` prints; there is nothing to configure. More on
sharing a Mac's devices with a team: [docs/device-farm.md](docs/device-farm.md).

The mirror has a few quirks a web test wouldn't expect — only the current screen is mirrored, fields
are real `<input>`s, typing is verified. See [docs/behaviors.md](docs/behaviors.md).

### Record a flow

Press **Record**, use the app, and press **Stop**. DeviceDeck writes what you did as a Maestro flow:
each tap is addressed by the most durable selector on screen (an id first, then text), graded for
how likely it is to survive the next build. Right-click an element while recording to **assert it is
visible** or **wait until it is visible**.

```yaml
# devicedeck: tapOn selector=id confidence=high
- tapOn:
    id: "login-button"
    label: "login-button"
# devicedeck: extendedWaitUntil visible selector=id confidence=high
- extendedWaitUntil:
    visible:
      id: "products-screen"
    timeout: 10000
```

Passwords are never written into the flow: a password field becomes `${PASSWORD_INPUT}`, supplied
when you replay. Screens with no durable ids are flagged at the top of the flow, with how to add
them. Copy or save the flow, then replay it with
[maestro-runner](https://github.com/devicelab-dev/maestro-runner):

```bash
maestro-runner --driver devicelab test flow.yaml -e PASSWORD_INPUT=…
```

The same file runs unchanged on real devices at [devicelab.dev](https://devicelab.dev).
[`examples/captured/`](examples/captured/) has recorded flows.

## Scope

Simulators and emulators only — no real hardware, no camera, biometrics or carrier. Within that,
the ceiling is physics, not an artificial limit.

## Known limits

- **The server is unauthenticated.** It listens on all interfaces (`0.0.0.0:8787`) by default, so
  anyone on your network can view and drive your devices. That suits a trusted office or home
  network; on shared Wi-Fi run `devicedeck --addr 127.0.0.1:8787` to keep it to this Mac. There is
  no access control yet — do not put it on the open internet as-is.
- **A driver that dies without closing its connection holds its device for up to half a minute**,
  until a missed ping releases it — see [one device, one worker](#with-your-tests).
- **Android video is ~18 fps and heavier than iOS.** The emulator's gRPC screenshot stream offers no
  video codec, so every frame is a full PNG rather than an H.264 delta. Emulators DeviceDeck boots
  run headless, because macOS throttles an occluded window — and the emulator's window is occluded
  exactly when you are watching the browser.
- **A freshly launched app swallows touches for about a second** after its screen is already in the
  accessibility tree. `POST /app/launch` waits this window out, so a test that launches through it
  can act at once; a raw terminate-and-launch outside it cannot.
- **A raw relaunch does not reset app state** — a native app stays logged in across
  terminate-and-launch. `POST /app/launch` wipes the app's data first *by default*, starting at a
  first-run screen; pass `?reset=no` to resume where it was left.
- **Two-finger gestures are dropped on Android.** They work on iOS; the Android driver has no mapping
  for them, so they are discarded rather than guessed at.

## Troubleshooting

`devicedeck doctor` checks the tools DeviceDeck needs: Xcode, the iOS runtime, adb, an Android
emulator, Node.js, Claude Code and maestro-runner.

Every run writes a folder under `~/.devicedeck/logs` (the path is printed at startup):
`devicedeck.log` with every request, device event and tool call, `runner.log` from the device
driver, one log per sidecar and device, and `crash.log` if the process panics. The terminal shows
only what needs attention; `DEVICEDECK_LOG=info` or `debug` shows more there too. The last 20 runs
are kept — attach the folder to an issue.

## Licence

Apache License 2.0 — see [`LICENSE`](LICENSE).

## Built on

[maestro-runner](https://github.com/devicelab-dev/maestro-runner) for the device drivers, Apache-2.0.
The Swift sidecars derive from [baguette](https://github.com/tddworks/baguette) (Apache-2.0) and
[tapflow](https://github.com/jo-duchan/tapflow) (MIT); [`ATTRIBUTION.md`](ATTRIBUTION.md) records what
was reused and where it lives.

<p align="center">
  <a href="https://devicelab.dev"><img src="docs/devicelab-logo.png" width="34" alt="DeviceLab"></a>
  <br>
  Built by <a href="https://devicelab.dev"><strong>devicelab.dev</strong></a>
</p>
