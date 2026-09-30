import { test } from '@playwright/test';
import { APP, UDID, openReady } from '../support/device';

// The seed test for Playwright's test agents (`npx playwright init-agents`):
// the planner and generator run it first to reach a known start, then explore
// from the page it leaves open. Here that start is the app launched fresh on
// the device — data wiped, first screen — so every plan begins from the same
// place a real run does.
test('seed', async ({ page }) => {
  await openReady(page, UDID, `app=${APP}&reset=yes`);
});
