import { test, expect } from '@playwright/test';
import { deviceValue, launchApp, openDevice } from '../support/device';

// Recorded, not written. The three getByTestId lines below are the
// verbatim output of `playwright-cli recording-start`, driving TestHive's
// login by hand in the DeviceDeck mirror on an iOS simulator, then
// `recording-stop`. Nothing about them says "mobile": the recorder saw a
// web page and emitted the locators it emits for any page, because the
// mirror is one. That is the proof — a stock web-automation recorder
// produces a durable, reviewable mobile test with no plugin. The raw
// transcript is under examples/agents/playwright-cli/; the driver-by-
// driver results are in docs/agents.md.
test('a stock Playwright recording logs into TestHive by testid', async ({ page, request }) => {
  await launchApp(request);
  await openDevice(page);

  // --- recorded verbatim by playwright-cli ---
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');
  // A DeviceDeck field's fill() returns when the mirror has the text and
  // the keystrokes are still travelling to the device; every replay waits
  // for the device to echo them before the next step (docs/behaviors.md).
  // This one poll is the only addition to the recording.
  await expect.poll(() => deviceValue(request, 'username-input')).toBe('devicelab');
  await page.getByTestId('login-button').click();
  // --- end recorded ---

  await expect(page.getByTestId('cart-button')).toBeVisible();
  await expect(page.getByTestId('add-to-cart-1')).toBeVisible();
});
