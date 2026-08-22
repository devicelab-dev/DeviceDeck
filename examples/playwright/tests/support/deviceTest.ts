import { test as base, expect } from '@playwright/test';

// Device-aware test: attaches the real device screen to any test that
// fails, so a failure is diagnosable without streaming video.
//
// App tests drive off the DOM mirror (the tree), not the pixels, so the
// automation page runs with no video — which means a normal Playwright
// screenshot of the page shows a blank canvas, useless at failure time.
// Instead, on failure only, this fetches a screenshot straight from the
// device (GET /screenshot, a simctl capture) and attaches it to the
// report and trace. On-demand, not a continuous stream: no encoder, no
// bandwidth, and none of the load a live video feed costs a headless run.
const UDID = process.env.DEVICEDECK_UDID || 'booted';

export const test = base.extend<{ deviceShotOnFailure: void }>({
  deviceShotOnFailure: [
    async ({ request }, use, testInfo) => {
      await use();
      if (testInfo.status === testInfo.expectedStatus) return;
      try {
        const res = await request.get(`/api/devices/${UDID}/screenshot`);
        if (res.ok()) {
          await testInfo.attach('device-screen-at-failure', {
            body: await res.body(),
            contentType: 'image/png',
          });
        }
      } catch {
        // A device that cannot be screenshotted is already the failure
        // the test is reporting; swallow so we do not mask it.
      }
    },
    { auto: true },
  ],
});

export { expect };
