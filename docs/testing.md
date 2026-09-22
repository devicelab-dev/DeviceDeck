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

## Drive it

```ts
await page.goto(`/device/${UDID}?app=${APP}`);
await page.getByTestId('username-input').fill('devicelab');
await page.getByTestId('login-button').click();
await expect(page.getByTestId('cart-button')).toBeVisible();
```

- **Typing:** `page.keyboard.type(text, { delay: 150 })` — keystrokes go in as real HID presses.
- **Wait for the echo before submitting:** the device's own value is exposed on
  `data-dd-device-value` (a secure field reports bullets — check length).
- **First render** waits on the tree-engine warm-up, so give the first selector ~30s.
- **Native gestures** a DOM event can't express: `page.evaluate(() => devicedeck.gesture('home'))` —
  see [behaviors](behaviors.md).

## One device, one worker

A device takes one driver at a time: two clients tapping at once would interleave into nonsense, so
a second one is refused and told who holds the device. Test runners go parallel by default, so set
one worker per device (`workers: 1` in Playwright) — and to run in parallel, boot more devices and
give each worker its own. A tab left open on the device in the [console](console.md) counts as a
driver too.

## Frameworks

Runnable projects, each with a login + checkout journey:
[`examples/playwright`](../examples/playwright) · [`examples/cypress`](../examples/cypress) ·
[`examples/puppeteer`](../examples/puppeteer). They drive the same DOM; only the idiom differs —
Playwright auto-waits, Cypress retries via `.should()`, Puppeteer waits manually.
