import { defineConfig } from '@playwright/test';

// The device page updates its DOM mirror on a short poll of the native UI
// tree, so timings differ from a plain webpage: actions land instantly,
// but new screens appear on the next mirror refresh (~300ms).
export default defineConfig({
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
