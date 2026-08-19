import { test, expect } from '@playwright/test';

// Same journey, same selectors, other platform: the device page mirrors
// the Android emulator's native tree, so this reads identically to the
// iOS spec — React Native's testID reaches both mirrors as data-testid.
// Requires `devicedeck serve`, a running emulator with TestHive
// installed, and DEVICEDECK_ANDROID_SERIAL (e.g. emulator-5554).

const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL;
const APP = 'com.testhiveapp';

test.skip(!SERIAL, 'set DEVICEDECK_ANDROID_SERIAL to run the Android variant');

// Start from the app's first screen rather than inheriting whatever the
// device was last left showing — which, after a reboot, is the launcher.
test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${SERIAL}/app/launch`, { data: { app: APP } });
  await new Promise((r) => setTimeout(r, 3000));
});

test('logs into TestHive on an Android emulator', async ({ page }) => {
  await page.goto(`/device/${SERIAL}`);

  const username = page.getByTestId('username-input');
  await expect(username).toBeVisible({ timeout: 30_000 });

  await username.click();
  await page.keyboard.type('devicelab', { delay: 120 });

  await page.getByTestId('password-input').click();
  await page.keyboard.type('robustest', { delay: 120 });

  await page.getByTestId('login-button').click();

  await expect(page.getByText('Hello, devicelab!')).toBeVisible({ timeout: 15_000 });
});
