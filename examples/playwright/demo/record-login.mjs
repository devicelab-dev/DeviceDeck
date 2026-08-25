// Record the demo: stock Playwright driving a real simulator through the
// DOM mirror. Opens the device page with ?video=1 so the recording shows
// the actual device screen (H.264) with the mirror driving it, logs into
// TestHive at a human pace, and saves a .webm. Convert to a GIF with the
// one-liner in demo/README.md.
//
//   DEVICEDECK_UDID=<booted-udid> node demo/record-login.mjs
//
// Needs `devicedeck serve` up and TestHive installed on the booted device.
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';

const BASE = process.env.DEVICEDECK_URL || 'http://127.0.0.1:8787';
const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';
const OUT = new URL('./out/', import.meta.url).pathname;
mkdirSync(OUT, { recursive: true });

const pause = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await chromium.launch();
// A tall, phone-ish viewport so the recording frames the device, not a
// desktop page with a small mirror in the corner.
const context = await browser.newContext({
  viewport: { width: 520, height: 980 },
  recordVideo: { dir: OUT, size: { width: 520, height: 980 } },
});
const page = await context.newPage();

// ?video=1: show the device's own screen behind the mirror, so the
// recording is the phone being driven, not an abstract DOM.
await page.goto(`${BASE}/device/${UDID}?app=${APP}&video=1`);
await page.getByTestId('username-input').waitFor({ state: 'visible', timeout: 30_000 });
await pause(1200);

// Typed at a readable pace — the point of the demo is to watch the device
// react to stock web-driver calls.
await page.getByTestId('username-input').click();
await page.keyboard.type('devicelab', { delay: 140 });
await pause(500);
await page.getByTestId('password-input').click();
await page.keyboard.type('robustest', { delay: 140 });
await pause(600);

// Wait for the device to echo the values before submitting (see the login
// examples for why), then sign in.
await page.waitForFunction(() => {
  const v = (id) => document.querySelector(`[data-testid="${id}"]`)?.dataset.ddDeviceValue || '';
  return v('username-input') === 'devicelab' && v('password-input').length === 'robustest'.length;
}, { timeout: 10_000 }).catch(() => {});
await page.getByTestId('login-button').click();

await page.getByText('Hello, devicelab!').waitFor({ timeout: 20_000 });
await pause(1500);

await context.close(); // finalizes the .webm
await browser.close();
console.log('recorded → demo/out/  (convert to GIF: see demo/README.md)');
