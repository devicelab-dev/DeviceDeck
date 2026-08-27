# Writing tests

The device is a web page at `/device/{udid}?app={bundleId}`, so **Playwright, Cypress, or Puppeteer
drive it with ordinary selectors** — the app's accessibility ids become `data-testid`, roles and
labels become ARIA. Nothing mobile-specific.

## Setup

- Point your framework's `baseURL` at `http://127.0.0.1:8787`.
- Run a **single worker** — a device serves one driver at a time.
- **Launch the app fresh in a fixture** so the first action lands:
  ```ts
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
  ```
  It blocks until the app is *taking input* — absorbing the boot, the engine warm-up, and the
  post-launch tap-swallow window. Pass `{ app, reset: false }` to resume instead of a first-run screen.

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

## Frameworks

Runnable projects, each with a login + checkout journey:
[`examples/playwright`](../examples/playwright) · [`examples/cypress`](../examples/cypress) ·
[`examples/puppeteer`](../examples/puppeteer). They drive the same DOM; only the idiom differs —
Playwright auto-waits, Cypress retries via `.should()`, Puppeteer waits manually.
