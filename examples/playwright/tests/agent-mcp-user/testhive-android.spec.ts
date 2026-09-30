import { test as base, expect, type Page } from '@playwright/test';

// TestHive (com.testhiveapp) on an Android emulator, driven through DeviceDeck.
// Every test starts from a fresh launch (app data cleared), so each one is
// independent and begins on the login screen.

const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL || 'emulator-5554';
const APP = 'com.testhiveapp';
const USER = 'devicelab';
const PASSWORD = 'robustest';

const test = base.extend<{ device: Page }>({
  device: async ({ page, request }, use) => {
    const res = await request.post(`/api/devices/${SERIAL}/app/launch`, { data: { app: APP } });
    expect(res.ok(), await res.text()).toBeTruthy();
    await page.goto(`/device/${SERIAL}?app=${APP}`);
    // First render waits on the tree-engine warm-up.
    await expect(page.getByTestId('login-screen')).toBeVisible({ timeout: 30_000 });
    await use(page);
  },
});

/** Types into a field with real HID pacing and waits until the device echoes it back. */
async function typeInto(page: Page, testId: string, text: string, secure = false) {
  await page.getByTestId(testId).click();
  await page.keyboard.type(text, { delay: 150 });
  await page.waitForFunction(
    ({ id, want, secure }) => {
      const v = document.querySelector(`[data-testid="${id}"]`)?.getAttribute('data-dd-device-value') ?? '';
      return secure ? v.length === want.length : v === want;
    },
    { id: testId, want: text, secure },
  );
}

/**
 * Closes the soft keyboard with the keyboard's Enter (submit) key. On the
 * checkout screens a tap made while the keyboard is up only dismisses the
 * keyboard, so the button under it never fires — close it first.
 */
async function submitField(page: Page, testId: string) {
  await page.keyboard.press('Enter');
  await expect(page.getByTestId(testId)).toHaveAttribute('data-dd-focused', 'false');
}

async function login(page: Page, user = USER, password = PASSWORD) {
  await typeInto(page, 'username-input', user);
  await typeInto(page, 'password-input', password, true);
  await page.getByTestId('login-button').click();
}

async function loginToProducts(page: Page) {
  await login(page);
  await expect(page.getByTestId('products-screen')).toBeVisible();
}

test.describe('TestHive Android', () => {
  test('rejects a wrong password', async ({ device: page }) => {
    await login(page, USER, 'wrong');
    await expect(page.getByTestId('error-message')).toHaveText('Invalid username or password');
    await expect(page.getByTestId('products-screen')).toHaveCount(0);
  });

  test('logs in and shows the product catalogue', async ({ device: page }) => {
    await loginToProducts(page);
    await expect(page.getByText(`Hello, ${USER}!`)).toBeVisible();
    for (const name of ['Appium', 'Maestro', 'Espresso', 'Selenium', 'Cypress']) {
      await expect(page.getByTestId(`product-item-${name}`)).toBeVisible();
    }
  });

  test('search narrows the catalogue', async ({ device: page }) => {
    await loginToProducts(page);
    await typeInto(page, 'search-input', 'Mae');
    await expect(page.getByTestId('product-item-Maestro')).toBeVisible();
    await expect(page.getByTestId('product-item-Appium')).toHaveCount(0);
    await expect(page.getByTestId('product-item-Espresso')).toHaveCount(0);
  });

  test('category filter shows only that category', async ({ device: page }) => {
    await loginToProducts(page);
    await page.getByRole('button', { name: 'Web Testing' }).click();
    await expect(page.getByTestId('product-item-Selenium')).toBeVisible();
    await expect(page.getByTestId('product-item-Cypress')).toBeVisible();
    await expect(page.getByTestId('product-item-Appium')).toHaveCount(0);
    await expect(page.getByTestId('product-item-Espresso')).toHaveCount(0);

    await page.getByRole('button', { name: 'All' }).click();
    await expect(page.getByTestId('product-item-Appium')).toBeVisible();
  });

  test('product detail shows the product and goes back', async ({ device: page }) => {
    await loginToProducts(page);
    await page.getByTestId('product-item-Maestro').click();
    const detail = page.getByTestId('product-detail-screen');
    await expect(detail).toBeVisible();
    await expect(detail).toContainText('$5.99');
    await expect(detail).toContainText('Mobile Testing');
    await expect(detail).toContainText('In Stock');

    await page.getByTestId('back-button').click();
    await expect(page.getByTestId('products-screen')).toBeVisible();
  });

  test('cart adds, increments, removes and totals items', async ({ device: page }) => {
    await loginToProducts(page);
    await page.getByTestId('add-to-cart-button-Maestro').click();
    await page.getByTestId('add-to-cart-button-Espresso').click();
    await expect(page.getByTestId('cart-button')).toContainText('2');

    await page.getByTestId('cart-button').click();
    await expect(page.getByTestId('cart-screen')).toBeVisible();
    await expect(page.getByText('My Cart (2 items)')).toBeVisible();
    await expect(page.getByTestId('cart-total-text')).toHaveText('$7.49');

    await page.getByTestId('add-to-cart-button-Maestro').click();
    await expect(page.getByTestId('quantity-text-Maestro')).toHaveText('2');
    await expect(page.getByTestId('cart-total-text')).toHaveText('$13.48');

    await page.getByTestId('remove-from-cart-button-Espresso').click();
    await expect(page.getByTestId('cart-item-Espresso')).toHaveCount(0);
    await expect(page.getByTestId('cart-total-text')).toHaveText('$11.98');
    await expect(page.getByText('My Cart (2 items)')).toBeVisible();
  });

  test('checkout end to end places an order and empties the cart', async ({ device: page }) => {
    // Four typed fields at real HID pacing — this journey runs close to a minute.
    test.slow();
    await loginToProducts(page);
    await page.getByTestId('add-to-cart-button-Cypress').click();
    await page.getByTestId('add-to-cart-button-Espresso').click();
    await page.getByTestId('cart-button').click();
    await expect(page.getByTestId('cart-total-text')).toHaveText('$9.60');
    await page.getByTestId('checkout-button').click();

    await expect(page.getByTestId('address-screen')).toBeVisible();
    await typeInto(page, 'address-input', '1 Infinite Loop');
    await typeInto(page, 'city-input', 'Cupertino');
    await typeInto(page, 'zip-input', '95014');
    await submitField(page, 'zip-input');
    await page.getByTestId('next-button').click();

    await expect(page.getByTestId('payment-screen')).toBeVisible();
    await typeInto(page, 'card-number-input', '4111111111111111');
    await submitField(page, 'card-number-input');
    await page.getByTestId('pay-now-button').click();

    const success = page.getByTestId('checkout-success-screen');
    await expect(success).toBeVisible();
    await expect(success).toContainText('Order Successful!');
    await expect(success).toContainText(/#\d+/);
    await expect(success).toContainText('$9.60');

    await page.getByTestId('back-to-home-button').click();
    await expect(page.getByTestId('products-screen')).toBeVisible();
    await expect(page.getByTestId('cart-button')).not.toContainText(/\d/);
  });

  test('logout asks for confirmation and returns to login', async ({ device: page }) => {
    await loginToProducts(page);

    await page.getByTestId('menu-button').click();
    await page.getByRole('button', { name: 'LOGOUT' }).click();
    await expect(page.getByText('Are you sure you want to logout?')).toBeVisible();
    await page.getByRole('button', { name: 'CANCEL' }).click();
    await expect(page.getByTestId('products-screen')).toBeVisible();

    await page.getByTestId('menu-button').click();
    await page.getByRole('button', { name: 'LOGOUT' }).click();
    await expect(page.getByText('Are you sure you want to logout?')).toBeVisible();
    await page.getByRole('button', { name: 'LOGOUT' }).click();
    await expect(page.getByTestId('login-screen')).toBeVisible();
  });
});
