import { test } from '@playwright/test';
const UDID = process.env.DEVICEDECK_UDID!;
const APP = 'dev.devicelab.testhive';
test('agent snapshot + ref addressing', async ({ page }) => {
  test.setTimeout(120000);
  await page.goto(`/device/${UDID}?app=${APP}`);
  await page.getByTestId('username-input').waitFor({ state: 'visible', timeout: 30000 });
  await page.waitForTimeout(1500);

  // What playwright-mcp actually calls for its snapshot.
  const anyPage = page as any;
  const hasAI = typeof anyPage._snapshotForAI === 'function';
  console.log('HAS _snapshotForAI:', hasAI);
  let snap = '';
  if (hasAI) snap = await anyPage._snapshotForAI();
  else snap = await page.locator('body').ariaSnapshot();
  const lines = snap.split('\n');
  const refLines = lines.filter(l => /\[ref=/.test(l));
  console.log('SNAPSHOT lines:', lines.length, '| lines carrying [ref=]:', refLines.length);
  console.log('SAMPLE:\n' + lines.slice(0, 14).join('\n'));
});
