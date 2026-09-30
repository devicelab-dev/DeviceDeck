import { test } from '@playwright/test';
import { ANDROID, APP, UDID, openReady } from '../support/device';

// Two ways to keep a system permission prompt from breaking a test.

// 1. Decide the permission up front: ?grant= (or ?revoke=) sets it before the
//    app launches, so the prompt never appears. Deterministic — prefer it.
test('grant up front', async ({ page }) => {
  // Android grants only permissions the app declares, and TestHive's Android
  // build declares none: the page would rightly report the grant as refused.
  test.skip(ANDROID && !process.env.DEVICEDECK_APP, 'TestHive declares no runtime permissions on Android');
  await openReady(page, UDID, `app=${APP}&reset=yes&grant=location,photos`);
});

// 2. Answer the prompt whenever it shows: a system alert is mirrored as an
//    alertdialog, so Playwright's locator handler taps Allow before any
//    action it would block. Use it for prompts ?grant= cannot set, such as
//    notifications on iOS.
test('answer prompts as they come', async ({ page }) => {
  await page.addLocatorHandler(page.getByRole('alertdialog'), async (dialog) => {
    await dialog.getByRole('button', { name: /^(Allow|Allow While Using App|OK)$/ }).first().click();
  });
  await openReady(page, UDID, `app=${APP}&reset=yes`);
});
