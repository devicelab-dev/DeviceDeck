---
name: devicedeck-maestro
description: Use when writing a Maestro flow (Maestro YAML test) for an iOS simulator or Android emulator app with DeviceDeck. Explore the live app through DeviceDeck, verify every selector on the device as you go, and emit a flow by accessibility id that replays unchanged on devicelab.dev real devices with maestro-runner. No Maestro CLI or Maestro Studio needed.
metadata:
  short-description: Write a Maestro flow for a mobile app with DeviceDeck
---

# Writing Maestro flows with DeviceDeck

DeviceDeck shows you the app's live screen — every element with its accessibility id, role, text
and position — and lets you act on it. You walk the user's journey on the device one step at a
time, and each step you perform becomes one line of a **Maestro flow**. Because every step ran on
the device before you wrote it, the flow is right the first time. It replays with
[maestro-runner](https://github.com/devicelab-dev/maestro-runner) locally and unchanged on
devicelab.dev real devices.

**Prerequisites** — the same as for a Playwright test (see `devicedeck-authoring`):
- `devicedeck` is running (default `http://127.0.0.1:8787`).
- Find the app with `list_apps` when the user says "my app"; boot a device yourself with
  `list_devices` → `boot_device` (poll `list_devices` until it is booted); `launch_app` with the
  bundle id. Launch blocks until the app takes input, and clears its data first.
- Do not use the Maestro CLI or its MCP server on a device DeviceDeck is driving: two drivers on
  one device fight, and both fail.

## Explore and act — every step verified on the device

Use DeviceDeck's MCP tools (or Playwright MCP on the device page — the same elements, where
`data-testid` is the accessibility id):

1. **`snapshot`** — the controls on screen, one per line:
   `e3 button "Sign In" testid=login-button @200,240`. `mode: "diff"` after an action shows only
   what changed; `mode: "full"` adds plain text (for assertions).
2. **Pick the selector for the step** (rules below) and **check it matches exactly one element**
   with `find_element` — Maestro taps the first match, so an ambiguous selector taps the wrong one.
   `find_element` matches text as a substring; Maestro's `text:` must match the whole label, so
   write the full label from the snapshot.
3. **Do the step on the device** — `tap`, `fill`, `long_press`, `swipe`, `press` — by the same
   selector you will write (`testid`, or `text`). If the tool refuses (off screen, under the
   keyboard or the status bar, disabled), the flow would fail there too: fix the step, not the
   tool call.
4. **Write the step**, then `snapshot` (or `wait_for`) to see the screen it led to, and continue.

Device state a journey needs — dark mode, a location, permissions — is set with
`device_settings`, not by tapping through the Settings app.

## Map what you saw to Maestro

| On the device | In the flow |
|---|---|
| `testid=login-button` | `- tapOn:` / `id: "login-button"` |
| no testid, visible text `Sign In` | `- tapOn: "Sign In"` (short form of `text:`) |
| several matches | narrow with `childOf: { id: "…" }` or `below: { id: "…" }` — `index:` only as a last resort |
| `fill` a field with `testid=email` | `- tapOn:` / `id: "email"` then `- inputText: "a@b.co"` |
| a password or other secret | `- inputText: ${PASSWORD}` and list it in a header comment |
| `press enter` | `- pressKey: Enter` |
| Android back button (no DeviceDeck tool) | `- back` |
| `swipe up` | `- scrollUntilVisible:` / `element: { id: "…" }` (not a fixed swipe) |
| `wait_for` visible | `- extendedWaitUntil:` / `visible: { id: "…" }` / `timeout: 10000` |
| `wait_for` gone | `- extendedWaitUntil:` / `notVisible: { id: "…" }` / `timeout: 10000` |
| what proves the step worked | `- assertVisible:` / `id: "…"` (or text) |
| `device_settings` location | `- setLocation:` / `latitude: 51.5` / `longitude: -0.12` |
| `open_url myapp://checkout` | `- openLink: myapp://checkout` |

`id:` and `text:` are **regular expressions matched against the whole value**: escape `.`, `(`,
`)`, `+`, `?`, `[`, `$` with a backslash, or a price like `$9.99` will not match.

A flow file:

```yaml
appId: com.example.app
# Needs: -e PASSWORD=…
---
- launchApp:
    clearState: true
- tapOn:
    id: "email-input"
- inputText: "user@example.com"
- tapOn:
    id: "password-input"
- inputText: ${PASSWORD}
- tapOn:
    id: "login-button"
- extendedWaitUntil:
    visible:
      id: "home-screen"
    timeout: 10000
- assertVisible: "Hello, user!"
```

## Rules that keep a flow durable

- **Accessibility id first, then visible text, never a point.** `tapOn: { point: "50%,40%" }`
  breaks on the next layout change and on another screen size. If an element has no id and no
  unique text, tell the user which element needs an accessibility identifier (React Native
  `testID`, SwiftUI `.accessibilityIdentifier`, Compose `Modifier.testTag`) instead of writing a
  point.
- **Only selector fields both platforms support:** `id`, `text`, `index`, `enabled`, `selected`,
  `focused`, `childOf`, `below`, `above`, `leftOf`, `rightOf`, `containsChild`,
  `containsDescendants`, `insideOf`, `width`, `height`. Not `css` or `checked` — iOS ignores them.
- **Start clean:** begin with `launchApp: clearState: true` so every run starts at the first-run
  screen, as the flow was written.
- **No fixed waits.** Never `- wait` or a sleep; wait for what the app shows with
  `extendedWaitUntil` (spinners, network) — the timeout is the limit, not a delay.
- **Assert the outcome** of each screen with `assertVisible` — a flow of taps proves the taps ran,
  not that the app worked.
- **Secrets never in the file:** `${VAR}` and the `-e` list in a header comment.
- **Reuse with `runFlow`:** a login used by several flows goes in its own file
  (`- runFlow: login.yaml`).

## Save and run

Save flows under `.maestro/` in the project (one journey per file, named for it:
`.maestro/login.yaml`). **Run them with maestro-runner** — never the Maestro CLI.

1. **Check it is installed:** `maestro-runner --version`. If the command is missing, **ask the user
   to install it** — do not install it yourself — with either:
   ```bash
   curl -fsSL https://open.devicelab.dev/install/maestro-runner | bash
   npm install --save-dev maestro-runner   # then run it as: npx maestro-runner
   ```
2. **Free the device.** A device takes one driver at a time, and DeviceDeck is driving the one
   you explored on. Run on another device DeviceDeck is not using, or hand this one over: end its
   DeviceDeck session with `POST /api/devices/{udid}/shutdown` (this powers it off), then let
   maestro-runner boot it — `--start-simulator <udid>` on iOS, `--start-emulator <avd name>` on
   Android (the `avd` field of `GET /api/devices`).
3. **Run it** with the devicelab driver (always pass it; older maestro-runner releases default to
   another driver on iOS). The global flags go before `test`, and `-e` goes **before the flow
   path** — after it, maestro-runner takes `-e` for a file name:
   ```bash
   maestro-runner --platform ios --device <udid> --driver devicelab test -e PASSWORD=… .maestro/login.yaml
   maestro-runner --platform android --device emulator-5554 --driver devicelab test .maestro/
   ```
4. **Read the result.** On a failed step, the report and the screenshot of the failure are in the
   output folder it prints. Fix the step on the device the same way you wrote it (steps 1–4
   above), then run again.

The same file runs on devicelab.dev real devices unchanged. Give the user the command you ran.

**Recorded instead of written:** a user can also record the journey in the DeviceDeck console
(Record → use the app → Stop); the flow comes out in this same shape with a confidence grade per
step — see `devicedeck-flows` for reviewing and improving a recording.
