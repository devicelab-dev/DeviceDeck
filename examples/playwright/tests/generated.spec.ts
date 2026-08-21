import { test, expect } from '@playwright/test';

// A test an agent generated.
//
// These action lines are verbatim from the "Ran Playwright code" sections
// playwright-mcp emitted during a live session on 2026-08-21, stitched
// together the way an agent assembles a spec. They ran fine inside MCP,
// which waits for the device after every action. Here they run as plain
// Playwright, which does not — so this is the test of whether a generated
// test works once it leaves the agent that wrote it.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

test.beforeEach(async ({ request }) => {
  await request.post(`/api/devices/${UDID}/app/launch`, { data: { app: APP } });
});

test('agent-generated: log in and add Maestro to the cart', async ({ page }) => {
  // Expected to fail until the product keeps the promise: fill() runs
  // inside the app's post-launch window and its focus tap is swallowed.
  // When this flips to an unexpected pass, delete the annotation.
  test.fail(true, 'post-launch tap window: see /app/launch readiness');
  await page.goto(`http://127.0.0.1:8787/device/${UDID}?app=${APP}`);
  await page.getByRole('textbox', { name: 'Username (username-input)' }).fill('devicelab');
  await page.getByRole('textbox', { name: 'Password (password-input)' }).fill('robustest');
  await page.getByRole('button', { name: 'Sign In (login-button)' }).click();
  await page.getByTestId('add-to-cart-2').click();
  await page.getByTestId('cart-button').click();
  await expect(page.getByText('Maestro')).toBeVisible();
});
