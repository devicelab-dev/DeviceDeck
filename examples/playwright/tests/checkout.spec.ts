import { test, expect } from '@playwright/test';

// A test case authored by driving playwright-mcp against TestHive.
//
// The action lines below are verbatim what MCP emitted, step by step,
// while an agent walked the journey — login, add a product, open the
// cart, and fill the shipping form through to payment. No line was
// hand-written; each is the durable getByTestId locator MCP produced
// for the element it acted on. The only additions are the assertions an
// author adds to make it a test rather than a recording, and the launch
// in beforeEach so it starts from a known screen.
//
// This is the product's whole claim, exercised end to end: a web
// developer's tooling authored a working mobile test, and it replays
// unchanged.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
});

test('checks out a product through the shipping form', async ({ page }) => {
  await page.goto(`http://127.0.0.1:8787/device/${UDID}?app=${APP}`);

  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');
  await page.getByTestId('login-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();

  await page.getByTestId('add-to-cart-2').click();
  await page.getByTestId('cart-button-cart-button').click();
  await expect(page.getByText('Maestro')).toBeVisible();

  await page.getByTestId('checkout-button').click();
  await page.getByTestId('address-input').fill('1 Market St');
  await page.getByTestId('city-input').fill('San Francisco');
  await page.getByTestId('zip-input').fill('94105');

  // The form gates Next until it is filled — the mirror reports native
  // enablement, so this is a real check that the fields landed.
  const next = page.getByTestId('next-payment-button');
  await expect(next).toBeEnabled();
  await next.click();

  // Past the shipping step: the order flow offers to continue shopping.
  await expect(page.getByTestId('continue-shopping-button')).toBeVisible();
});
