# Writing tests

The device is a web page at `/device/{udid}?app={bundleId}`, so **Playwright, Cypress, or Puppeteer
drive it with ordinary selectors** — the app's accessibility ids become `data-testid`, roles and
labels become ARIA. Nothing mobile-specific.

## Getting a first test

You don't need a mobile test suite to start:

- **Let your agent write it** — the four steps in the [README](../README.md#get-started).
- **Record it.** Playwright's [playwright-cli](https://github.com/microsoft/playwright-cli) records
  what you do on the device page (`recording-start` … `recording-stop`) and writes ordinary
  locators — here is [a recorded login](../examples/playwright/tests/recorded/login-recorded.spec.ts).
  Add one wait for the device to echo typed text before submitting (see *Drive it* below).
- **Start from an example** — copy a project from [`examples/`](../examples/) and point it at your app.

## Setup

- Point your framework's `baseURL` at `http://127.0.0.1:8787`.
- Run **one worker per device** — see below.
- **Launch the app fresh in a fixture** so the first action lands:
  ```ts
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
  ```
  It blocks until the app is *taking input* — absorbing the boot, the engine warm-up, and the
  post-launch tap-swallow window. It clears the app's data first, so the test starts at a
  first-run screen; post to `/app/launch?reset=no` to resume where the app was left instead.

- **Let Playwright start DeviceDeck**, if you installed it with npm:
  ```ts
  webServer: { command: 'npx devicedeck --app path/to/MyApp.app', url: 'http://127.0.0.1:8787', reuseExistingServer: true },
  ```

## Drive it

```ts
await page.goto(`/device/${UDID}?app=${APP}`);
await page.getByTestId('username-input').fill('devicelab');
await page.getByTestId('login-button').click();
await expect(page.getByTestId('cart-button')).toBeVisible();
```

- **Typing:** `locator.fill(text)`. It returns once the device holds the value, so there is nothing
  to wait for before submitting. Avoid `keyboard.type`: key by key it raises Android's soft keyboard,
  and the next tap can be spent closing it. The device's own read-back of a field is on
  `data-dd-device-value` (a secure field reports bullets).
- **The device in a spec:** read it from an environment variable (`DEVICEDECK_UDID`) rather than
  hard-coding the UDID of your simulator, so the spec runs on a teammate's Mac and in CI.
- **Don't use `page.clock`:** the app runs on the device's clock, which Playwright's fake clock
  cannot reach, and pausing it would also pause the page's own refresh of the device screen.
- **First render** waits on the tree-engine warm-up, so give the first selector ~30s.
- **Device state from the URL:** `?appearance=dark`, `?location=51.5074,-0.1278`, and with `?app=`,
  `?grant=location,photos` / `?revoke=camera` set the device up before the app launches; `?link=myapp://checkout`
  opens a deep link after. The page does this before the mirror shows anything, and a failure is shown
  on the page:
  ```ts
  await page.goto(`/device/${UDID}?app=${APP}&reset=yes&appearance=dark&grant=location`);
  ```
  Mid-test, `page.evaluate(() => devicedeck.settings({ appearance: 'light' }))` and
  `devicedeck.openURL(url)` do the same. Permission names: `location`, `microphone`, `contacts`,
  `photos`, `calendar`, `motion` on both platforms; `reminders` on iOS; `camera` and `notifications`
  on Android (the simulator has no camera). iOS ends the app when some permissions change.
- **Native gestures** a DOM event can't express, on `window.devicedeck`:
  `page.evaluate(() => devicedeck.gesture('home'))` — also `swipe`, `button`, `key` and `screenshot()`;
  see [behaviors](behaviors.md). The HTTP equivalents are in the [CLI reference](cli-reference.md#http-api).

## One device, one worker

A device takes one driver at a time: two clients tapping at once would interleave into nonsense, so
a second one is refused and told who holds the device. Test runners go parallel by default, so set
one worker per device (`workers: 1` in Playwright) — and to run in parallel, boot more devices and
give each worker its own. A tab left open on the device in the [console](console.md) counts as a
driver too. An HTTP action (`/tap`, `/act`, …) from another client while a page holds the
device is still carried out — an agent may tap through DeviceDeck's MCP tools while its browser tool
holds the page — but it is logged and its response carries an `X-DeviceDeck-Warning` header.

## Recipes

Short specs in [`examples/playwright/tests/recipes`](../examples/playwright/tests/recipes/):

- **[Accessibility check](../examples/playwright/tests/recipes/a11y.spec.ts)** — run
  [axe](https://github.com/dequelabs/axe-core-npm/tree/develop/packages/playwright) on `#mirror`
  with its naming rules: the mirror carries the app's own accessibility data, so an unnamed control
  in the app is an axe violation.
- **[Permission prompts](../examples/playwright/tests/recipes/permission-prompts.spec.ts)** — set
  the permission up front with `?grant=`, or answer a system alert (mirrored as an `alertdialog`)
  with `page.addLocatorHandler`.
- **[Two devices in one test](../examples/playwright/tests/recipes/two-devices.spec.ts)** — a page
  per device, driven together; the same selectors work on iOS and Android.
- **[Seed test for Playwright's test agents](../examples/playwright/tests/recipes/seed.spec.ts)** —
  the start `npx playwright init-agents` plans and generates from: the app launched fresh.

## Frameworks

Runnable projects, each with a login + checkout journey:
[`examples/playwright`](../examples/playwright) · [`examples/cypress`](../examples/cypress) ·
[`examples/puppeteer`](../examples/puppeteer). They drive the same DOM; only the idiom differs —
Playwright auto-waits, Cypress retries via `.should()`, Puppeteer waits manually.
