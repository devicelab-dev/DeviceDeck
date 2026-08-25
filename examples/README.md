# Drive a phone with your web stack

The device page (`/device/{udid}?app={bundleId}`) is a **real webpage**: DeviceDeck mirrors the
native UI tree into the DOM, so a browser-automation tool drives the simulator/emulator with the
*same code it uses for a website* — `getByTestId`, `[data-testid=…]`, click, type. No mobile driver,
no new DSL, no coordinates.

Each tool ships two runnable journeys against TestHive — **log in** and **check out through the
shipping form** — written the same way, so you can compare the tools side by side. They all assume
`devicedeck serve` is running and a device is booted with the app installed and freshly launched.

| Tool | Path | Locator style |
| --- | --- | --- |
| **Playwright** | [`playwright/`](playwright/) | `page.getByTestId('username-input')` |
| **Cypress** | [`cypress/`](cypress/) | `cy.get('[data-testid="username-input"]')` |
| **Puppeteer** | [`puppeteer/`](puppeteer/) | `page.waitForSelector('[data-testid="username-input"]')` |

They share one contract:

- **Same URL** — `/device/${DEVICEDECK_UDID}?app=dev.devicelab.testhive` (Android: the emulator serial).
- **Same credentials** — `devicelab` / `robustest`.
- **Same selectors** — the app's own accessibility identifiers become `data-testid`; roles and labels
  become ARIA, so `getByRole`/`getByLabel`/`getByText` work too.
- **Keys forward as real HID presses** on the device (~100ms hold), so pace typing if a tool types
  faster than the app can accept.
- **Wait for the device to echo a value before a submit that depends on it.** Keys land on the
  device a moment after the DOM has them, so a submit fired the instant typing "finished" can race
  the last keystrokes. Playwright and Cypress fold that wait into their auto-retrying assertions;
  the Puppeteer examples ask for it explicitly, and read the field back via `data-dd-device-value`.
  A new screen appears on the mirror's next poll, so wait for an element before acting on it —
  Playwright/Cypress do this for you; Puppeteer's `waitForSelector` is the same idea.

`captured/` holds Maestro flows recorded from a manual session — the Flow Capture output that runs
unchanged on real devices. That is the other direction: record on a simulator, replay anywhere.
