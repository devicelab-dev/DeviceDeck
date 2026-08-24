# Drive a phone with your web stack

The device page (`/device/{udid}?app={bundleId}`) is a **real webpage**: DeviceDeck mirrors the
native UI tree into the DOM, so a browser-automation tool drives the simulator/emulator with the
*same code it uses for a website* — `getByTestId`, `[data-testid=…]`, click, type. No mobile driver,
no new DSL, no coordinates.

Each example logs into TestHive on a booted device the same way, so you can compare the tools side
by side. They all assume `devicedeck serve` is running and a device is booted with the app installed
and freshly launched.

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

`captured/` holds Maestro flows recorded from a manual session — the Flow Capture output that runs
unchanged on real devices. That is the other direction: record on a simulator, replay anywhere.
