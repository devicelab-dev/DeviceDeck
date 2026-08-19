import { defineConfig } from '@playwright/test';

// The device page updates its DOM mirror on a short poll of the native UI
// tree, so timings differ from a plain webpage: actions land instantly,
// but new screens appear on the next mirror refresh (~300ms).
export default defineConfig({
  testDir: './tests',
  // A device serves one driver at a time. Playwright parallelises across
  // files by default, which points two specs at the same simulator and
  // has them fight over it — each passes alone and the pair fails
  // together. DeviceDeck does not yet refuse a second claim, so the
  // constraint has to be honoured here.
  workers: 1,
  fullyParallel: false,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: process.env.DEVICEDECK_URL || 'http://127.0.0.1:8787',
    trace: 'retain-on-failure',
  },
});
