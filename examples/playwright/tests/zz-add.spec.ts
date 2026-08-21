import { test } from '@playwright/test';
const UDID = '56B22693-2704-4861-8DCF-9DD2ED0483FA';
test('can an agent tell the Add buttons apart', async ({ page }) => {
  await page.goto(`/device/${UDID}?app=dev.devicelab.testhive`);
  await page.waitForTimeout(6000);
  const snap = await page.locator('#stage').ariaSnapshot();
  const adds = snap.split('\n').filter(l => /button "Add/.test(l)).map(l => l.trim());
  console.log('ADD BUTTONS:'); adds.forEach(a => console.log('   ' + a));
  console.log('unique names: ' + new Set(adds).size + ' of ' + adds.length);
  // the decisive test: can it address Maestro's Add by name alone?
  const maestro = page.getByRole('button', { name: 'add-to-cart-2' });
  console.log('addressable by identifier: ' + await maestro.count());
});
