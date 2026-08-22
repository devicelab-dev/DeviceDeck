// Fresh-agent benchmark: author test cases against TestHive knowing only
// the login credentials, discovering every element from what is on
// screen — never a selector learned in a previous run. This is the
// honest measure of whether the mirror is legible to an agent, and it is
// meant to be re-run after each change to DeviceDeck to see whether
// authoring got better or worse.
//
// Usage:
//   DEVICEDECK_UDID=<udid> node tests/support/discover.mjs
//   DEVICEDECK_UDID=<udid> npx playwright test generated-agent
//
// The only knowledge given: username "devicelab", password "robustest".
// Everything else — which field is the username, which button adds to the
// cart, how to reach checkout — is read from the live snapshot by role and
// visible name, the way an agent reasons over the page.
import { spawn, execFileSync } from 'node:child_process';
import { writeFileSync } from 'node:fs';

const UDID = process.env.DEVICEDECK_UDID;
const APP = 'dev.devicelab.testhive';
const APP_PATH = process.env.TESTHIVE_APP || '/Users/omnarayan/work/temp/TestHive-iOS/prebuilt-ios/simulator/testhive.app';
const PAGE = `http://127.0.0.1:8787/device/${UDID}?app=${APP}`;
const USER = 'devicelab';
const PASS = 'robustest';

// resetApp returns the app to a clean slate: a plain relaunch keeps
// TestHive logged in (iOS persists app data across terminate+launch), so
// each journey would inherit the last one's state and a fresh agent
// would never see the login screen again. Uninstall+reinstall is the
// true blank slate, so every journey starts where a first-time user
// does. This is what the ?reset=1 page mode will do for real tests.
function resetApp() {
  try {
    execFileSync('xcrun', ['simctl', 'uninstall', UDID, APP], { stdio: 'ignore' });
  } catch {}
  execFileSync('xcrun', ['simctl', 'install', UDID, APP_PATH], { stdio: 'ignore' });
}

const srv = spawn('npx', ['-y', '@playwright/mcp@latest', '--headless'], { stdio: ['pipe', 'pipe', 'ignore'] });
let buf = '', id = 0;
const pending = new Map();
srv.stdout.on('data', (d) => {
  buf += d;
  for (let nl; (nl = buf.indexOf('\n')) >= 0; ) {
    const l = buf.slice(0, nl).trim();
    buf = buf.slice(nl + 1);
    if (!l) continue;
    let m;
    try { m = JSON.parse(l); } catch { continue; }
    if (m.id != null && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  }
});
const rpc = (method, params) => new Promise((res, rej) => {
  const i = ++id;
  pending.set(i, (m) => (m.error ? rej(new Error(JSON.stringify(m.error))) : res(m.result)));
  srv.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: i, method, params }) + '\n');
  setTimeout(() => rej(new Error('timeout ' + method)), 120000);
});
const txt = (r) => (r?.content || []).map((c) => c.text || '').join('\n');
const codeOf = (r) => { const m = txt(r).match(/```js\n([\s\S]*?)```/); return m ? m[1].trim() : ''; };
const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

let emitted = [];
const call = async (n, a) => { const r = await rpc('tools/call', { name: n, arguments: a }); const c = codeOf(r); if (c) emitted.push(c); return txt(r); };
const snapshot = () => call('browser_snapshot', {});
const settle = (secs) => rpc('tools/call', { name: 'browser_wait_for', arguments: { time: secs } });

// find waits for a live, addressable element matching role + a
// case-insensitive substring of its visible name, retrying through the
// settle animation that briefly strips refs. This retry is the ref
// recovery a real agent gets for free ("Ref not found, capture new
// snapshot"); without it a naive driver reads a ref-less instant.
async function find(role, nameSub, { nth = 0, tries = 10 } = {}) {
  const re = new RegExp(`${role} "([^"]*${esc(nameSub)}[^"]*)" \\[ref=([^\\]]+)\\]`, 'gi');
  let lastSnap = '';
  for (let t = 0; t < tries; t++) {
    const s = await snapshot();
    lastSnap = s;
    const hits = [...s.matchAll(re)];
    if (hits.length > nth) return { ref: hits[nth][2], name: hits[nth][1] };
    await settle(1.5);
  }
  const controls = lastSnap.split('\n').filter((l) => /button |textbox |heading /.test(l)).slice(0, 12).map((l) => l.trim());
  throw new Error(`could not find ${role} ~"${nameSub}"\n  screen shows:\n    ${controls.join('\n    ')}`);
}
// allOf returns every current match — used to fill an unknown form.
async function allOf(role, nameSub = '') {
  const s = await snapshot();
  const re = new RegExp(`${role} "([^"]*${esc(nameSub)}[^"]*)" \\[ref=([^\\]]+)\\]`, 'gi');
  return [...s.matchAll(re)].map((m) => ({ name: m[1], ref: m[2] }));
}

const type = async (role, nameSub, text) => { const e = await find(role, nameSub); await call('browser_type', { element: e.name, target: e.ref, text }); };
const click = async (role, nameSub, opts) => { const e = await find(role, nameSub, opts); await call('browser_click', { element: e.name, target: e.ref }); return e; };

// A plausible value for a discovered field, from its own label.
function valueFor(name) {
  const n = name.toLowerCase();
  if (/zip|postal|code/.test(n)) return '94105';
  if (/city/.test(n)) return 'San Francisco';
  if (/address|street/.test(n)) return '1 Market St';
  if (/name/.test(n)) return 'Test User';
  if (/email/.test(n)) return 'test@example.com';
  return 'Test';
}

async function freshLogin() {
  resetApp();
  await fetch(`http://127.0.0.1:8787/api/devices/${UDID}/app/launch`, { method: 'POST', body: JSON.stringify({ app: APP }) });
  await call('browser_navigate', { url: PAGE });
  await type('textbox', 'Username', USER);
  await type('textbox', 'Password', PASS);
  await click('button', 'Sign In');
}

const results = [];
async function journey(name, fn, assertion) {
  emitted = [];
  try {
    const asrt = await fn();
    results.push({ name, ok: true, lines: [...emitted], assertion: assertion || asrt });
    console.log(`  PASS  generated "${name}" (${emitted.length} action lines)`);
  } catch (e) {
    results.push({ name, ok: false, error: String(e.message).split('\n')[0] });
    console.log(`  FAIL  "${name}": ${e.message}`);
  }
}

await rpc('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'discover', version: '1' } });

// Journey A: just log in. Success = the store is reachable (a cart control
// appears), inferred, not memorised.
await journey('log in', async () => {
  await freshLogin();
  await find('button', 'cart');
  return "await expect(page.getByRole('button', { name: /cart/i }).first()).toBeVisible();";
});

// Journey B: log in, add the first product, open the cart. Success = a
// checkout control appears.
await journey('add a product to the cart', async () => {
  await freshLogin();
  await click('button', 'Add');            // first Add button on the list
  await click('button', 'cart');           // the badge that appears
  await find('button', 'Checkout');
  return "await expect(page.getByRole('button', { name: /checkout/i })).toBeVisible();";
});

// Journey C: log in, add, open cart, checkout, fill whatever the form
// asks for, proceed. Success = the flow moved past the address form.
await journey('check out through the shipping form', async () => {
  await freshLogin();
  await click('button', 'Add');
  await click('button', 'cart');
  await click('button', 'Checkout');
  await find('textbox', '');               // wait for the form
  for (const field of await allOf('textbox')) {
    await call('browser_type', { element: field.name, target: field.ref, text: valueFor(field.name) });
  }
  const proceed = await click('button', 'Next').catch(() => click('button', 'Payment'));
  await settle(2);
  // Assert on a control that appeared after proceeding, discovered live.
  const end = (await allOf('button')).map((b) => b.name).find((n) => /continue|shopping|order|done|home/i.test(n));
  return end
    ? `await expect(page.getByRole('button', { name: ${JSON.stringify(end)} })).toBeVisible();`
    : "await expect(page.getByRole('button', { name: /payment|place|pay/i }).first()).toBeVisible();";
});

srv.kill();

const gotoLine = 'await page.goto(`http://127.0.0.1:8787/device/${UDID}?app=${APP}`);';
const clean = (l) => (l.includes('setTimeout') ? null : l.startsWith('await page.goto(') ? gotoLine : l);
const block = (r) => {
  const steps = r.lines.map(clean).filter(Boolean).map((l) => '  ' + l).join('\n');
  return `test('agent: ${r.name}', async ({ page }) => {\n${steps}\n  ${r.assertion}\n});`;
};

const ok = results.filter((r) => r.ok);
const body = `import { test, expect } from '@playwright/test';

// GENERATED by a fresh agent (tests/support/discover.mjs) that knew only
// the login credentials and found every element on screen by role and
// name. Re-generate after a DeviceDeck change and compare pass counts.

const UDID = process.env.DEVICEDECK_UDID || 'booted';
const APP = 'dev.devicelab.testhive';

import { execFileSync } from 'child_process';

const APP_PATH = process.env.TESTHIVE_APP || '/Users/omnarayan/work/temp/TestHive-iOS/prebuilt-ios/simulator/testhive.app';

// Each test starts from a clean install so it is independent: a plain
// relaunch keeps TestHive logged in, so without this the second test on
// would never see the login screen. (A ?reset=1 page mode would move
// this server-side; until then the reset is done here.)
test.beforeEach(async ({ request }) => {
  try { execFileSync('xcrun', ['simctl', 'uninstall', UDID, APP]); } catch {}
  execFileSync('xcrun', ['simctl', 'install', UDID, APP_PATH]);
  await request.post(\`/api/devices/\${UDID}/app/launch\`, { data: { app: APP } });
});

${ok.map(block).join('\n\n')}
`;
writeFileSync('tests/generated-agent.spec.ts', body);

console.log('\n=== BENCHMARK: fresh-agent authoring ===');
console.log(`journeys attempted: ${results.length}`);
console.log(`generated cleanly:  ${ok.length}`);
for (const r of results) console.log(`  ${r.ok ? 'ok  ' : 'FAIL'} ${r.name}${r.ok ? '' : ' — ' + r.error}`);
console.log(`\nwrote tests/generated-agent.spec.ts (${ok.length} tests)`);
process.exit(0);
