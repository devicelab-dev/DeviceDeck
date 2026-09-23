import { test, expect } from '@playwright/test';

// Written by an agent (Claude) from the code Playwright MCP 0.0.82 emitted
// while it explored the WebdriverIO native demo app (MIT,
// github.com/webdriverio/native-demo-app) with no prior knowledge of it.
// Same journey on both platforms; the locators differ because the app's
// accessibility tree differs per platform (see DEVICE-TEST-PROGRESS).
const IOS = process.env.DEVICEDECK_UDID || 'booted';
const ANDROID = process.env.DEVICEDECK_ANDROID_SERIAL || 'emulator-5554';

test('mcp agent (iOS): log in through the Login tab', async ({ page }) => {
  await page.goto(`/device/${IOS}?app=org.wdiodemoapp&reset=yes`);
  await page.getByRole('button', { name: 'Login' }).click();
  await page.getByTestId('input-email').fill('test@webdriver.io');
  await page.getByTestId('input-password').fill('Test1234!');
  await page.getByTestId('button-LOGIN').getByLabel('LOGIN').click();
  await expect(page.getByText('You are logged in!')).toBeVisible();
  await page.getByRole('button', { name: 'OK' }).click();
  await expect(page.getByText('You are logged in!')).toBeHidden();
});

test('mcp agent (Android): log in through the Login tab', async ({ page }) => {
  await page.goto(`/device/${ANDROID}?app=com.wdiodemoapp&reset=yes`);
  await page.getByRole('button', { name: 'Login' }).click();
  await page.getByRole('textbox', { name: 'input-email (RNE__Input__text' }).fill('test@webdriver.io');
  await page.getByRole('textbox', { name: 'input-password (' }).fill('Test1234!');
  await page.getByRole('button', { name: 'button-LOGIN', exact: true }).click();
  await expect(page.getByText('You are logged in!')).toBeVisible();
  await page.getByTestId('button1').click();
  await expect(page.getByText('You are logged in!')).toBeHidden();
});
