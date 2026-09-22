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

[Get started](#get-started) · [What it does](#what-it-does) · [Using it](#using-it) · [Docs](docs/) · [Known limits](#known-limits)

</div>

---

## Get started

Needs a Mac with Xcode (and the Android SDK for Android) — `devicedeck doctor` says what's missing.

```bash
# 1. Install, then start DeviceDeck with your app build (.app for a simulator, .apk for an emulator)
curl -fsSL https://open.devicelab.dev/install/devicedeck | bash
export PATH="$HOME/.devicedeck/bin:$PATH"   # or open a new terminal
devicedeck --app path/to/MyApp.app

# 2. Give Claude Code the browser tool and DeviceDeck's plugin
claude mcp add playwright npx @playwright/mcp@latest
claude plugin marketplace add devicelab-dev/DeviceDeck
claude plugin install devicedeck@devicedeck-marketplace

# 3. Ask Claude — it boots a simulator, launches your app, drives it, writes the test
#    "Write a Playwright test that logs in to my app com.your.app"

# 4. Run it
npx playwright test
```

Using Gemini CLI, Codex, VS Code or Cursor? Only step 2 changes — see [Other agents](#other-agents).
Prefer to use devices by hand? Open the console at `http://127.0.0.1:8787`.

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

## Using it

### How the agent drives it

The agent drives the device the way it drives any web page: it **snapshots the page**, reasons over
the tree, and acts by ref — no coordinates, no vision model. Here it picks the right **Add** among
five identical ones:

<div align="center">
<img src="docs/demo-agent.gif" width="820" alt="An AI agent using Playwright MCP: browser_snapshot returns the product screen as an accessibility tree with five identical Add buttons, a reasoning step picks the one after Maestro (add-to-cart-2, ref e24), browser_click adds it, and opening the cart confirms Maestro was added — not Appium.">
</div>

### Other agents

In [Get started](#get-started), steps 1, 3 and 4 are the same; only step 2 changes.

| Agent | Step 2 |
|---|---|
| **Gemini CLI** | `gemini extensions install https://github.com/devicelab-dev/DeviceDeck` |
| **Codex CLI** | `codex mcp add playwright -- npx @playwright/mcp@latest`<br>`codex mcp add devicedeck -- devicedeck mcp`<br>`npx skills add devicelab-dev/DeviceDeck` |
| **VS Code / Copilot** | `code --add-mcp '{"name":"playwright","command":"npx","args":["@playwright/mcp@latest"]}'`<br>`code --add-mcp '{"name":"devicedeck","command":"devicedeck","args":["mcp"]}'`<br>`npx skills add devicelab-dev/DeviceDeck` |
| **Cursor, and others** | add the same two MCP servers in the agent's settings, then `npx skills add devicelab-dev/DeviceDeck` — [docs/agents.md](docs/agents.md#set-up-your-agent) |

Each agent gets the same three pieces: **Playwright MCP** to drive the device page, DeviceDeck's
**device tools** to boot devices and launch apps, and its **skills** for writing tests, recording
flows and triaging failures.

### Already have tests, or prefer to write them?

Point Playwright, Cypress or Puppeteer at `http://127.0.0.1:8787/device/{udid}?app={bundleId}`,
run **one worker per device**, and select by `data-testid` (the app's accessibility id) or by role.
To record a test instead, or start from a template, see [docs/testing.md](docs/testing.md).

### By hand — the console

The console lists every simulator, emulator and `--app` build on the Mac. Pick a device to boot and
stream it, **Launch** an app onto it, and drive it with your mouse and keyboard; **Inspect** shows
each element's id, role and text — the selectors your tests use. Teammates open the network address
`devicedeck` prints. **Guide:** [docs/console.md](docs/console.md).

### Record a flow

Press **Record**, use the app, press **Stop**: DeviceDeck writes it as a
[Maestro](https://maestro.dev) flow with graded, durable selectors — replayable with
[maestro-runner](https://github.com/devicelab-dev/maestro-runner) and unchanged on real devices at
[devicelab.dev](https://devicelab.dev). **Guide:** [docs/flows.md](docs/flows.md).

## Install options

The install script puts DeviceDeck in `~/.devicedeck` and adds its `bin` folder to your `PATH` — no
sudo, and nothing else to install: the Android driver ships inside the binary. Pin a version with
`curl -fsSL https://open.devicelab.dev/install/devicedeck | bash -s -- --version 0.1.0`.

Or download an archive from [Releases](https://github.com/devicelab-dev/DeviceDeck/releases) and run
it in place (the two sidecars sit beside the binary in `bin/`):

```bash
tar xzf devicedeck-<version>-darwin-arm64.tar.gz
./devicedeck-<version>-darwin-arm64/bin/devicedeck
```

Or build from source (needs the Xcode toolchain for the Swift sidecars): `make sidecar && make build`.

`devicedeck` on its own starts the console at `http://127.0.0.1:8787`. `--app` takes `.app` simulator
builds, `.apk` files, or a folder of them, and installs each the first time it is launched on a
device. On start it prints the console link (and the network address teammates use), your apps, how
to connect Claude Code, and any missing tools with how to fix them.

Everything DeviceDeck writes lives in `~/.devicedeck` (set `DEVICEDECK_HOME` to move it). To
uninstall, delete that folder and the `# DeviceDeck` line from your shell profile.

## Requirements

- **A Mac to host.** iOS Simulators, `simctl`/CoreSimulator and the Swift sidecars are
  macOS-only, so the machine that runs the devices is a Mac. **Clients can be any OS:** the surface
  is a web page, so people, tests and agents drive it from Linux, Windows or another Mac.
- **Xcode** with at least one iOS Simulator runtime — **iOS 26.2 or newer is strongly
  recommended**; on 18.6 the simulator's render server crashes under repeated capture.
- **For Android:** the Android SDK, with `adb` and `emulator` on `PATH`, and at least one virtual
  device.

`devicedeck doctor` checks all of this and says how to fix anything missing.

## Status

Early release. Both platforms drive end to end: **Playwright, Cypress and Puppeteer each log into and
check out of TestHive through the same DOM**, and the console drives any device by hand. It is an
ordinary web page, so any browser driver works with no mobile-specific code. Typing is checked
against the device's own read-back, so a keystroke that does not land is retyped rather than lost —
on iOS *and* on Android's masked password fields.

It has run on a handful of Macs so far, so expect some first-contact problems — please
[open an issue](https://github.com/devicelab-dev/DeviceDeck/issues) with the log folder it prints.
See [**Known limits**](#known-limits).

## Scope

Simulators and emulators only — no real hardware, no camera, biometrics or carrier. Within that,
the ceiling is physics, not an artificial limit.

## Known limits

- **The server is unauthenticated.** It listens on all interfaces (`0.0.0.0:8787`) by default, so
  anyone on your network can view and drive your devices. That suits a trusted office or home
  network; on shared Wi-Fi run `devicedeck --addr 127.0.0.1:8787` to keep it to this Mac. There is
  no access control yet — do not put it on the open internet as-is.
- **A driver that dies without closing its connection holds its device for up to half a minute**,
  until a missed ping releases it — see [docs/testing.md](docs/testing.md#one-device-one-worker).
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
