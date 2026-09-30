import { test, expect } from '@playwright/test';

// Written by an agent (Claude) from the code Playwright MCP emitted while it
// explored TestHive on an iOS simulator, knowing only the login. Each
// action line is MCP's own output; the assertions are what the agent read
// in the snapshot after acting. Relative URL, so baseURL decides the
// server; every test starts from a fresh launch (logged out, empty cart).
const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';
const PAGE = `/device/${UDID}?app=${APP}`;

test.beforeEach(async ({ page, request }) => {
  const res = await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
  expect(res.ok()).toBeTruthy();
  await page.goto(PAGE);
});

async function logIn(page) {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('robustest');
  await page.getByTestId('login-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
}

async function addMaestroAndCypressThenOpenCart(page) {
  await logIn(page);
  await page.getByTestId('add-to-cart-2').click();
  await page.getByTestId('add-to-cart-5').click();
  await expect(page.getByTestId('increase-quantity-2')).toBeVisible();
  await expect(page.getByTestId('increase-quantity-5')).toBeVisible();
  await page.getByTestId('cart-button-cart-button').click();
  await expect(page.getByText('My Cart (2 items)')).toBeVisible();
}

test('mcp agent: wrong password shows an error', async ({ page }) => {
  await page.getByTestId('username-input').fill('devicelab');
  await page.getByTestId('password-input').fill('wrongpass');
  await page.getByTestId('login-button').click();
  const message = page.getByText("Please use valid credentials: • Username: 'devicelab' Password: 'robustest'");
  await expect(message).toBeVisible();
  await page.getByRole('button', { name: 'OK' }).click();
  await expect(message).toBeHidden();
  await expect(page.getByText('Welcome Back')).toBeVisible();
});

test('mcp agent: log in', async ({ page }) => {
  await logIn(page);
  await expect(page.getByTestId('search-bar')).toBeVisible();
});

test('mcp agent: filter to web testing', async ({ page }) => {
  await logIn(page);
  await page.getByRole('button', { name: 'Web Testing' }).click();
  await expect(page.getByText('Appium')).toBeHidden();
  await expect(page.getByText('Maestro')).toBeHidden();
  await expect(page.getByText('Selenium')).toBeVisible();
  await expect(page.getByText('Cypress')).toBeVisible();
  await expect(page.getByText('Playwright')).toBeVisible();
});

test('mcp agent: search the catalogue', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('search-bar').fill('Play');
  // The app debounces search; expect auto-waits for the list to settle.
  await expect(page.getByText('Appium')).toBeHidden();
  await expect(page.getByText('Selenium')).toBeHidden();
  await expect(page.getByText('Playwright')).toBeVisible();
  await expect(page.getByText('Unavailable')).toBeVisible();
});

test('mcp agent: add two products and check the cart total', async ({ page }) => {
  await addMaestroAndCypressThenOpenCart(page);
  await expect(page.getByText('$5.99 x 1 = $5.99')).toBeVisible();
  await expect(page.getByText('$8.10 x 1 = $8.10')).toBeVisible();
  await expect(page.getByText('$14.09', { exact: true })).toBeVisible();
});

test('mcp agent: change quantities and remove an item', async ({ page }) => {
  await addMaestroAndCypressThenOpenCart(page);
  await page.getByTestId('cart-increase-quantity-2').click();
  await expect(page.getByText('My Cart (3 items)')).toBeVisible();
  await expect(page.getByText('$5.99 x 2 = $11.98')).toBeVisible();
  await expect(page.getByText('$20.08', { exact: true })).toBeVisible();
  await page.getByTestId('cart-decrease-quantity-5').click();
  await expect(page.getByText('My Cart (2 items)')).toBeVisible();
  await expect(page.getByText('Cypress')).toBeHidden();
  await expect(page.getByText('$11.98', { exact: true })).toBeVisible();
});

test('mcp agent: full checkout', async ({ page }) => {
  await addMaestroAndCypressThenOpenCart(page);
  await page.getByTestId('cart-increase-quantity-2').click();
  await page.getByTestId('cart-decrease-quantity-5').click();
  await expect(page.getByText('$11.98', { exact: true })).toBeVisible();
  await page.getByTestId('checkout-button').click();
  await expect(page.getByTestId('next-payment-button')).toBeDisabled();
  await page.getByTestId('address-input').fill('1 Infinite Loop');
  await page.getByTestId('city-input').fill('Cupertino');
  await page.getByTestId('zip-input').fill('95014');
  await expect(page.getByTestId('next-payment-button')).toBeEnabled();
  await page.getByTestId('next-payment-button').click();
  // iOS TestHive has no payment form: it goes straight to processing.
  await expect(page.getByText('Order Successful!')).toBeVisible();
  await expect(page.getByText('$11.98', { exact: true })).toBeVisible();
  await page.getByTestId('continue-shopping-button').click();
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
  await expect(page.getByTestId('cart-button')).toBeVisible();
});

test('mcp agent: menu Logout closes the menu (iOS app stays signed in)', async ({ page }) => {
  await logIn(page);
  await page.getByTestId('line.horizontal.3').click();
  await expect(page.getByRole('button', { name: 'Profile' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Settings' })).toBeVisible();
  await page.getByRole('button', { name: 'Logout' }).click();
  await expect(page.getByRole('button', { name: 'Logout' })).toBeHidden();
  // TestHive iOS wires Logout to an empty action, so the user stays in.
  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});

test('mcp agent: forgot password shows the reset notice', async ({ page }) => {
  await page.getByTestId('forgot-password-button').click();
  const notice = page.getByText('Password reset functionality would be implemented here.');
  await expect(notice).toBeVisible();
  await page.getByRole('button', { name: 'OK' }).click();
  await expect(notice).toBeHidden();
  await expect(page.getByText('Welcome Back')).toBeVisible();
});
