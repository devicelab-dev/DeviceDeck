import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { APP, UDID, openReady } from '../support/device';

// An accessibility check of the app, with the tool web teams already use.
// The mirror carries the app's own accessibility data — roles, names,
// values — so axe's naming rules find the controls a screen reader (and an
// agent) cannot name. Only the mirror is checked, and only rules about the
// app's semantics: the page's title and landmarks are DeviceDeck's, and
// colours are the video's, which the transparent mirror does not have.
test('login screen: every control has a name', async ({ page }) => {
  await openReady(page, UDID, `app=${APP}&reset=yes`);
  await expect(page.getByTestId('login-button')).toBeVisible();

  const results = await new AxeBuilder({ page })
    .include('#mirror')
    .withRules(['button-name', 'link-name', 'image-alt', 'label', 'aria-toggle-field-name', 'aria-input-field-name'])
    .analyze();
  expect(results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(', ')}`)).toEqual([]);
});
