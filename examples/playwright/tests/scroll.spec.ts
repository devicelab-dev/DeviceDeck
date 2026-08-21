import { test, expect } from '@playwright/test';

// Scrolling the way a web test scrolls: page.mouse.wheel(). The mirror
// has no scrollable box, so this used to do nothing at all — the event
// fell on the page and the device never heard of it. Now it becomes a
// finger drag, and a product list moves the way a list does.
//
// Written without device helpers, like naive.spec.ts: what is asserted
// is the position of an element on the page, which is all a web test
// can see.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
});

test('the wheel scrolls a native list', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=${APP}`);
  await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
  await page.getByRole('textbox', { name: 'Password' }).fill('robustest');
  await page.getByRole('button', { name: 'Sign In' }).click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();

  const first = page.getByTestId('add-to-cart-1');
  const before = (await first.boundingBox())!;

  await page.mouse.wheel(0, 400);
  // The list moves up: the first row's box ends up above where it was.
  await expect.poll(async () => (await first.boundingBox())!.y).toBeLessThan(before.y - 50);

  await page.mouse.wheel(0, -400);
  // And back, to within a row's worth of where it started — a native
  // list snaps and overshoots, so exact equality is not the contract.
  await expect.poll(async () => Math.abs((await first.boundingBox())!.y - before.y)).toBeLessThan(40);
});

test('a click on a row below the fold scrolls it into view first', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=${APP}`);
  await page.getByRole('textbox', { name: 'Username' }).fill('devicelab');
  await page.getByRole('textbox', { name: 'Password' }).fill('robustest');
  await page.getByRole('button', { name: 'Sign In' }).click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();

  // The last product starts below the screen. A web test just clicks
  // it; the scroll-into-view Playwright does first is what brings it on.
  const device = (await page.locator('#mirror').boundingBox())!;
  const last = page.getByTestId('product-name-6');
  expect((await last.boundingBox())!.y).toBeGreaterThan(device.y + device.height);

  await last.click();

  const after = (await last.boundingBox())!;
  expect(after.y).toBeGreaterThanOrEqual(device.y);
  expect(after.y + after.height).toBeLessThanOrEqual(device.y + device.height + 1);
});
