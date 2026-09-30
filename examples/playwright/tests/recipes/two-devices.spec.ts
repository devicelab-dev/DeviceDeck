import { test, expect, type Page } from '@playwright/test';
import { openReady } from '../support/device';

// One test driving two devices at once — an iOS simulator and an Android
// emulator — each in its own page. The same selectors work on both because
// React Native's testID reaches both mirrors as data-testid. The pattern also
// fits two-party journeys (a chat, a ride and its driver).
const IOS = process.env.DEVICEDECK_UDID;
const ANDROID = process.env.DEVICEDECK_ANDROID_SERIAL;

test.skip(!IOS || !ANDROID, 'set DEVICEDECK_UDID and DEVICEDECK_ANDROID_SERIAL');

async function logIn(page: Page, device: string, app: string) {
  await openReady(page, device, `app=${app}&reset=yes`);
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');
  await page.getByTestId('login-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
}

test('log in on both platforms at once', async ({ browser }) => {
  // A page per device; separate contexts keep their input claims apart.
  const [ios, android] = await Promise.all([browser.newPage(), browser.newPage()]);
  await Promise.all([
    logIn(ios, IOS!, 'dev.devicelab.testhive'),
    logIn(android, ANDROID!, 'com.testhiveapp'),
  ]);
});
