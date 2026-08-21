import { test, expect } from '@playwright/test';

// A web developer's first draft.
//
// This is the acceptance test for the product's promise — that web
// automation habits work unchanged against a native app. It is written
// the way someone who has never seen a device would write it: fill() and
// click() and getByRole, no helpers, no waits, no retries. Nothing here
// may ever be made device-aware. If it fails, the product is what has to
// change, not this file.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
});

test('logs in and adds a product to the cart', async ({ page }) => {
  // Expected to fail until the product keeps the promise: fill() runs
  // inside the app's post-launch window and its focus tap is swallowed.
  // When this flips to an unexpected pass, delete the annotation.
  test.fail(true, 'post-launch tap window: see /app/launch readiness');
  await page.goto(`/device/${UDID}?app=${APP}`);

  await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
  await page.getByRole('textbox', { name: 'Password' }).fill('robustest');
  await page.getByRole('button', { name: 'Sign In' }).click();

  await expect(page.getByText('Hello, devicelab!')).toBeVisible();

  await page.getByTestId('add-to-cart-2').click();
  await page.getByTestId('cart-button').click();

  await expect(page.getByText('Maestro')).toBeVisible();
  await expect(page.getByText('Appium')).not.toBeVisible();
});
