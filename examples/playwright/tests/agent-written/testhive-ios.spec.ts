import { test, expect, Page } from '@playwright/test';

// TestHive (dev.devicelab.testhive) on an iOS simulator.
//
// Written by Claude through Playwright MCP, starting from nothing but "test my
// app" and the login: it booted the device, explored the app, and wrote this
// file. Every action line is code Playwright MCP emitted; the assertions are
// what its snapshots showed. Passed twice on DeviceDeck 1.0.1 as written.
const udid = process.env.DEVICEDECK_UDID || 'booted';
const app = 'dev.devicelab.testhive';

test.beforeEach(async ({ page, request }) => {
  // Fresh launch clears app data, so each journey starts at the login screen.
  const res = await request.post(`/api/devices/${udid}/app/launch`, { data: { app } });
  expect(res.ok()).toBeTruthy();
  await page.goto(`/device/${udid}?app=${app}`);
  await expect(page.getByTestId('username-input')).toBeVisible({ timeout: 30_000 });
});

async function login(page: Page, password = 'robustest') {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill(password);
  await expect(page.getByTestId('login-button')).toBeEnabled();
  await page.getByTestId('login-button').click();
}

test('login with valid credentials lands on the catalogue', async ({ page }) => {
  await expect(page.getByTestId('login-button')).toBeDisabled();
  await login(page);
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
  await expect(page.getByTestId('search-bar')).toBeVisible();
  await expect(page.getByText('Appium', { exact: true })).toBeVisible();
  await expect(page.getByTestId('add-to-cart-1')).toBeVisible();
});

test('login with a wrong password shows Invalid Credentials', async ({ page }) => {
  await login(page, 'wrongpass');
  await expect(page.getByText('Invalid Credentials').first()).toBeVisible();
  await page.getByRole('button', { name: 'OK' }).click();
  await expect(page.getByRole('button', { name: 'OK' })).toHaveCount(0);
  await expect(page.getByTestId('login-button')).toBeVisible();
  await expect(page.getByText('Hello, devicelab!')).toHaveCount(0);
});

test('search narrows the catalogue to matching frameworks', async ({ page }) => {
  await login(page);
  await expect(page.getByText('Appium', { exact: true })).toBeVisible();
  await page.getByTestId('search-bar').fill('Maes');
  await expect(page.getByText('Maestro', { exact: true })).toBeVisible();
  await expect(page.getByTestId('add-to-cart-2')).toBeVisible();
  await expect(page.getByText('Appium', { exact: true })).toHaveCount(0);
  await expect(page.getByTestId('add-to-cart-1')).toHaveCount(0);
});

test('category filter shows only Web Testing frameworks', async ({ page }) => {
  await login(page);
  await expect(page.getByText('Appium', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Web Testing' }).click();
  await expect(page.getByText('Selenium', { exact: true })).toBeVisible();
  await expect(page.getByText('Cypress', { exact: true })).toBeVisible();
  await expect(page.getByText('Playwright', { exact: true })).toBeVisible();
  await expect(page.getByText('Appium', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Espresso', { exact: true })).toHaveCount(0);
});

test('add to cart, adjust quantity and complete checkout', async ({ page }) => {
  await login(page);
  await page.getByTestId('add-to-cart-4').click();
  await page.getByTestId('add-to-cart-5').click();
  await page.getByTestId('increase-quantity-4').click();
  // The cart button's test id gains a badge suffix once the cart is non-empty.
  await expect(page.getByTestId('cart-button-cart-button')).toContainText('3');
  await page.getByTestId('cart-button-cart-button').click();

  await expect(page.getByText('My Cart (3 items)')).toBeVisible();
  await expect(page.getByText('$2.30 x 2 = $4.60')).toBeVisible();
  await expect(page.getByText('$8.10 x 1 = $8.10')).toBeVisible();
  await expect(page.getByText('$12.70')).toBeVisible();
  await page.getByTestId('checkout-button').click();

  await expect(page.getByText('Shipping Address')).toBeVisible();
  await expect(page.getByTestId('next-payment-button')).toBeDisabled();
  await page.getByTestId('address-input').fill('1 Infinite Loop');
  await page.getByTestId('city-input').fill('Cupertino');
  await page.getByTestId('zip-input').fill('95014');
  await expect(page.getByTestId('next-payment-button')).toBeEnabled();
  await page.getByTestId('next-payment-button').click();

  await expect(page.getByText('Order Successful!')).toBeVisible();
  await page.getByTestId('continue-shopping-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
  await expect(page.getByTestId('cart-button')).toBeVisible();
});
