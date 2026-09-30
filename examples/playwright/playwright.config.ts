import { defineConfig } from '@playwright/test';

// The device page mirrors the app's native UI as DOM. click() and fill()
// return once the device has acted and the screen has settled, so tests
// need no waits of their own; expect() still retries for anything the app
// changes later on its own timer.
export default defineConfig({
  // Every failed test gets the real device screen attached, for any test
  // regardless of how it imports `test` — no per-test fixture, no change
  // to what an agent generates. Captured on demand (no video). Shows in
  // the HTML report.
  reporter: [['list'], ['html', { open: 'never' }], ['./deviceScreenshotReporter.ts']],
  testDir: './tests',
  // A device serves one driver at a time. Playwright parallelises across
  // files by default, which points two specs at the same simulator;
  // DeviceDeck refuses the second claim, so the run fails loudly rather
  // than interleaving touches into nonsense. Loud is better than silent,
  // but it still fails — so keep the worker count at one.
  workers: 1,
  fullyParallel: false,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: process.env.DEVICEDECK_URL || 'http://127.0.0.1:8787',
    trace: 'retain-on-failure',
  },
});
