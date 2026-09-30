import { test, expect, type Page } from '@playwright/test';

// TestHive (com.testhiveapp) on an Android emulator.
//
// Written by Claude through Playwright MCP, starting from nothing but "test my
// app" and the login: it booted the device, explored the app, and wrote this
// file. Every action line is code Playwright MCP emitted; the assertions are
// what its snapshots showed. Passed twice on DeviceDeck 1.0.1 as written.
const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL || 'emulator-5554';
const APP = 'com.testhiveapp';

async function login(page: Page, password = 'robustest') {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill(password);
  await page.getByTestId('login-button').click();
}

test.beforeEach(async ({ page, request }) => {
  // Fresh launch (clears app data) so every test starts on the login screen.
  const res = await request.post(`/api/devices/${SERIAL}/app/launch`, { data: { app: APP } });
  expect(res.ok()).toBeTruthy();
  await page.goto(`/device/${SERIAL}?app=${APP}`);
  await expect(page.getByTestId('login-button')).toBeVisible({ timeout: 30_000 });
});

test('rejects a wrong password', async ({ page }) => {
  await login(page, 'wrongpass');
  await expect(page.getByText('Invalid username or password')).toBeVisible();
  await expect(page.getByTestId('login-button')).toBeVisible();
});

test('logs in and logs out', async ({ page }) => {
  await login(page);
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();

  await page.getByTestId('menu-button').click();
  await page.getByRole('button', { name: 'LOGOUT' }).click();
  await expect(page.getByText('Are you sure you want to logout?')).toBeVisible();
  await page.getByRole('button', { name: 'LOGOUT' }).click();

  await expect(page.getByText('Welcome Back')).toBeVisible();
  await expect(page.getByTestId('login-button')).toBeVisible();
});

test('search narrows the product list', async ({ page }) => {
  await login(page);
  await expect(page.getByTestId('product-item-Appium')).toBeVisible();

  await page.getByTestId('search-input').fill('Mae');
  await expect(page.getByTestId('product-item-Maestro')).toBeVisible();
  await expect(page.getByTestId('product-item-Appium')).toHaveCount(0);
  await expect(page.getByTestId('product-item-Cypress')).toHaveCount(0);
});

test('category filter shows only Web Testing products', async ({ page }) => {
  await login(page);
  await page.getByRole('button', { name: 'Web Testing' }).click();

  await expect(page.getByTestId('product-item-Selenium')).toBeVisible();
  await expect(page.getByTestId('product-item-Cypress')).toBeVisible();
  await expect(page.getByTestId('product-item-Playwright')).toContainText('Out of Stock');
  await expect(page.getByTestId('product-item-Appium')).toHaveCount(0);
  await expect(page.getByTestId('product-item-Maestro')).toHaveCount(0);
});

test('adds to cart and completes checkout', async ({ page }) => {
  await login(page);
  await page.getByRole('button', { name: 'Web Testing' }).click();
  await page.getByTestId('add-to-cart-button-Selenium').click();
  await expect(page.getByTestId('cart-button')).toContainText('1');
  await page.getByTestId('add-to-cart-button-Selenium').click();
  await expect(page.getByTestId('cart-button')).toContainText('2');
  await page.getByTestId('add-to-cart-button-Cypress').click();
  await expect(page.getByTestId('cart-button')).toContainText('3');

  await page.getByTestId('cart-button').click();
  await expect(page.getByText('My Cart (3 items)')).toBeVisible();
  await expect(page.getByText('$2.30 × 2 = $4.60')).toBeVisible();
  await expect(page.getByText('$8.10 × 1 = $8.10')).toBeVisible();
  await expect(page.getByText('$12.70')).toBeVisible();

  await page.getByTestId('checkout-button').click();
  await expect(page.getByText('Shipping Address')).toBeVisible();
  await page.getByTestId('address-input').fill('1 Infinite Loop');
  await page.getByTestId('city-input').fill('Cupertino');
  await page.getByTestId('zip-input').fill('95014');
  await page.getByTestId('next-button').click();

  await expect(page.getByText('Credit Card Number')).toBeVisible();
  await page.getByTestId('card-number-input').fill('4111111111111111');
  await page.getByTestId('pay-now-button').click();

  await expect(page.getByText('Order Successful!')).toBeVisible();
  await expect(page.getByText('$12.70')).toBeVisible();
  await page.getByTestId('back-to-home-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
