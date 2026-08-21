import { test, expect } from '@playwright/test';
import { clearField, fillField, launchApp, openDevice } from './support/device';

// The device page's whole promise is that ordinary web selectors work
// against a native app. Our other specs use getByTestId, which exercises
// only one of them — and that blind spot let a bug ship where typing
// renamed a field and broke every role- and label-based query without a
// single test noticing. This spec drives the same journey using the
// query styles a real user reaches for first.

test.beforeEach(async ({ request }) => {
  await launchApp(request);
});

test('drives TestHive by role, label and text', async ({ page }) => {
  await openDevice(page);

  // A control's identifier is folded into its accessible name, so a name
  // query matches on the part the app author wrote. getByRole matches a
  // substring by default, which is what keeps "Username" working against
  // "Username (username-input)".
  const username = page.getByRole('textbox', { name: 'Username' });
  await expect(username).toBeVisible({ timeout: 30_000 });

  // Sign In is disabled until credentials exist — the mirror has to
  // report native enablement, not just draw a button.
  const signIn = page.getByRole('button', { name: 'Sign In' });
  await expect(signIn).toBeDisabled();

  await clearField(page, username);
  await fillField(page, username, 'devicelab');

  // The name must survive typing: it comes from what the field asks for,
  // never from what has been entered.
  await expect(page.getByRole('textbox', { name: 'Username' })).toHaveCount(1);

  await fillField(page, page.getByRole('textbox', { name: 'Password' }), 'robustest');

  await expect(signIn).toBeEnabled();
  await signIn.click();

  await expect(page.getByText('Hello, devicelab!')).toBeVisible();
});
