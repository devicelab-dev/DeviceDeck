import { defineConfig } from '@playwright/test';

// The device page updates its DOM mirror on a short poll of the native UI
// tree, so timings differ from a plain webpage: actions land instantly,
// but new screens appear on the next mirror refresh (~300ms).
export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: process.env.DEVICEDECK_URL || 'http://127.0.0.1:8787',
    trace: 'retain-on-failure',
  },
});
