import { test } from '@playwright/test';
test('android fps timeline', async ({ page }) => {
  test.setTimeout(240000);
  const start = Date.now();
  const frames: { t: number; type: number; len: number }[] = [];
  page.on('websocket', ws => {
    if (!ws.url().includes('/video')) return;
    ws.on('framereceived', f => { const b = f.payload as Buffer; frames.push({ t: Date.now()-start, type: b[0], len: b.length }); });
  });
  await page.goto('/?device=emulator-5554');
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  const box0 = await page.locator('#video').boundingBox();
  const mid = { x: box0!.x + box0!.width/2, y: box0!.y + box0!.height/2 };
  // scroll continuously for 30s so the screen is always changing
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    await page.mouse.move(mid.x, mid.y + 150);
    await page.mouse.down();
    for (let s = 0; s < 5; s++) await page.mouse.move(mid.x, mid.y + 150 - s*45, { steps: 2 });
    await page.mouse.up();
    await page.waitForTimeout(200);
  }
  const buckets: Record<string, number> = {};
  for (const f of frames) { const b = Math.floor(f.t/5000)*5; buckets[`${b}-${b+5}s`] = (buckets[`${b}-${b+5}s`]||0)+1; }
  const types = frames.reduce((m: any, f) => { const k='0x'+f.type.toString(16); m[k]=(m[k]||0)+1; return m; }, {});
  console.log('TOTAL frames:', frames.length, 'over ~30s =>', (frames.length/30).toFixed(1), 'fps');
  console.log('TYPES:', JSON.stringify(types));
  console.log('PER 5s BUCKET:', JSON.stringify(buckets));
  console.log('avgKB:', frames.length ? Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024) : 0);
});
