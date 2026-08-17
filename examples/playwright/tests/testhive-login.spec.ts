import { test, expect } from '@playwright/test';

// The device is a normal webpage: locate native elements with standard
// Playwright selectors against the DOM mirror, and drive the simulator
// through them. Requires `devicedeck serve` running with a booted
// simulator that has TestHive installed.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

test('logs into TestHive on a real simulator', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=${APP}`);

  // First mirror render needs the tree engine warm; give it time.
  const username = page.getByTestId('username-input');
  await expect(username).toBeVisible({ timeout: 30_000 });

  await username.click();
  // Keys forward as real HID presses (~100ms hold each) — pace the typing.
  await page.keyboard.type('devicelab', { delay: 150 });

  await page.getByTestId('password-input').click();
  await page.keyboard.type('robustest', { delay: 150 });

  await page.getByTestId('login-button').click();

  // Post-login home screen: mirror text comes from native labels.
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
