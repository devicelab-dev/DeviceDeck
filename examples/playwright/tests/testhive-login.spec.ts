import { test, expect } from '@playwright/test';
import { APP, UDID, clearField, deviceValue, fillField, launchApp, openDevice } from './support/device';

// The device is a normal webpage: locate native elements with standard
// Playwright selectors against the DOM mirror, and drive the simulator
// through them. Requires `devicedeck serve` running with a booted
// simulator that has TestHive installed.

// Each spec begins from the app's first screen. Without this they share
// one device and the second to run inherits wherever the first left it —
// which is exactly how running the suite together failed while each spec
// passed alone.
test.beforeEach(async ({ request }) => {
  await launchApp(request);
});

test('logs into TestHive on a real simulator', async ({ page, request }) => {
  await openDevice(page);

  // Relaunching does not reset app state — a field can still hold what a
  // previous session typed — so clear it, which doubles as proof that
  // the app is taking input at all before anything is typed for real.
  const username = page.getByTestId('username-input');
  await expect(username).toBeVisible({ timeout: 30_000 });
  await clearField(page, username);

  await fillField(page, username, 'devicelab');
  await fillField(page, page.getByTestId('password-input'), 'robustest');

  // Assert against the device, not the mirror: the mirror showing text
  // the device never received is the exact failure this suite exists to
  // catch, and it is invisible to a check that reads the input back.
  // Polled, because fill() returns when the mirror has the text and the
  // keystrokes are still on their way to the device behind it.
  await expect.poll(() => deviceValue(request, 'username-input')).toBe('devicelab');

  await page.getByTestId('login-button').click();

  // Post-login home screen: mirror text comes from native labels.
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
