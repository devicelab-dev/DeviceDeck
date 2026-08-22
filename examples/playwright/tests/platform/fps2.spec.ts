import { test } from '@playwright/test';
test('android fps long', async ({ page }) => {
  test.setTimeout(180000);
  const frames: { t: number; type: number; len: number }[] = [];
  const start = Date.now();
  page.on('websocket', ws => {
    if (!ws.url().includes('/video')) return;
    ws.on('framereceived', f => { const b = f.payload as Buffer; frames.push({ t: Date.now()-start, type: b[0], len: b.length }); });
  });
  await page.goto('/?device=emulator-5554');
  await page.locator('#console-view').waitFor({ state: 'visible', timeout: 20000 });
  await page.waitForFunction(() => {
    const c = document.getElementById('video') as HTMLCanvasElement;
    return c && !(c.width === 300 && c.height === 150);
  }, { timeout: 40000 });
  // give the encoder a generous warm-up before measuring
  await page.waitForTimeout(6000);
  frames.length = 0;
  const t0 = Date.now();
  const box = (await page.locator('#video').boundingBox())!;
  const mid = { x: box.x + box.width/2, y: box.y + box.height/2 };
  for (let i = 0; i < 10; i++) {
    await page.mouse.move(mid.x, mid.y + 150); await page.mouse.down();
    for (let s = 0; s < 6; s++) await page.mouse.move(mid.x, mid.y + 150 - s*40, { steps: 2 });
    await page.mouse.up(); await page.waitForTimeout(300);
  }
  await page.waitForTimeout(2000);
  const secs = (Date.now() - t0) / 1000;
  const types = frames.reduce((m: any, f) => { const k='0x'+f.type.toString(16); m[k]=(m[k]||0)+1; return m; }, {});
  console.log(`RESULT frames=${frames.length} over ${secs.toFixed(1)}s => ${(frames.length/secs).toFixed(1)} fps | avgKB=${frames.length?Math.round(frames.reduce((a,f)=>a+f.len,0)/frames.length/1024):0} | types=${JSON.stringify(types)}`);
});
