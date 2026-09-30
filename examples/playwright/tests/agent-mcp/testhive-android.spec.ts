import { test, expect } from '@playwright/test';

// Written by an agent (Claude) from the code Playwright MCP emitted while it
// explored TestHive on an Android emulator, knowing only the login. Every
// action line is MCP's own output; the assertions are what the agent read
// in the snapshot after acting. Relative URL, so baseURL decides the server.
const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL || 'emulator-5554';
const APP = 'com.testhiveapp';
const PAGE = `/device/${SERIAL}?app=${APP}`;

// Every test starts from a fresh app: the launch wipes its data, so each
// begins logged out with an empty cart.
test.beforeEach(async ({ page, request }) => {
  await request.post(`/api/devices/${SERIAL}/app/launch`, { data: { app: APP } });
  await page.goto(PAGE);
});

async function logIn(page) {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');
  await page.getByTestId('login-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
}

test('mcp agent: rejects a wrong password', async ({ page }) => {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('wrongpass');
  await page.getByTestId('login-button').click();
  await expect(page.getByText('Invalid username or password')).toBeVisible();
});

test('mcp agent: logs in', async ({ page }) => {
  await logIn(page);
});

test('mcp agent: filters to web testing', async ({ page }) => {
  await logIn(page);
  await page.getByRole('button', { name: 'Web Testing' }).click();
  await expect(page.getByTestId('product-item-Selenium')).toBeVisible();
  await expect(page.getByTestId('product-item-Cypress')).toBeVisible();
  await expect(page.getByTestId('product-item-Playwright')).toBeVisible();
  await expect(page.getByTestId('product-item-Appium')).toBeHidden();
});

test('mcp agent: searches the catalogue', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('search-input').fill('Esp');
  // The app filters 300ms after typing stops; expect waits it out.
  await expect(page.getByTestId('product-item-Espresso')).toBeVisible();
  await expect(page.getByTestId('product-item-Appium')).toBeHidden();
  await page.getByTestId('search-input').fill('');
  await expect(page.getByTestId('product-item-Appium')).toBeVisible();
});

test('mcp agent: changes quantities in the cart', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('add-to-cart-button-Espresso').click();
  await page.getByTestId('add-to-cart-button-Maestro').click();
  await page.getByTestId('cart-button').click();
  await expect(page.getByText('My Cart (2 items)')).toBeVisible();
  await expect(page.getByText('$7.49')).toBeVisible();
  await page.getByTestId('add-to-cart-button-Maestro').click();
  await expect(page.getByText('$5.99 × 2 = $11.98')).toBeVisible();
  await expect(page.getByText('$13.48')).toBeVisible();
  await page.getByTestId('remove-from-cart-button-Espresso').click();
  await expect(page.getByText('My Cart (2 items)')).toBeVisible();
  await expect(page.getByText('Espresso')).toBeHidden();
});

test('mcp agent: checks out', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('add-to-cart-button-Appium').click();
  await page.getByTestId('add-to-cart-button-Maestro').click();
  await page.getByTestId('cart-button').click();
  await expect(page.getByText('$13.49')).toBeVisible();
  await page.getByTestId('checkout-button').click();
  await page.getByTestId('address-input').fill('42 Elm Street');
  await page.getByTestId('city-input').fill('Austin');
  await page.getByTestId('zip-input').fill('73301');
  await page.getByTestId('next-button').click();
  await page.getByTestId('card-number-input').fill('4242424242424242');
  await page.getByTestId('pay-now-button').click();
  await expect(page.getByText('Order Successful!')).toBeVisible();
  await expect(page.getByText('$13.49')).toBeVisible();
  await page.getByTestId('back-to-home-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});

test('mcp agent: logs out from the menu', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('menu-button').click();
  await page.getByRole('button', { name: 'LOGOUT' }).click();
  await expect(page.getByText('Are you sure you want to logout?')).toBeVisible();
  await page.getByRole('button', { name: 'LOGOUT' }).click();
  await expect(page.getByText('Welcome Back')).toBeVisible();
});
