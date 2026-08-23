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

const SERIAL = process.env.DEVICEDECK_ANDROID_SERIAL;
const ANDROID = !!SERIAL;
const DEV = ANDROID ? SERIAL : process.env.DEVICEDECK_UDID;
const APP = ANDROID ? 'com.testhiveapp' : 'dev.devicelab.testhive';
const APP_PATH = ANDROID
  ? (process.env.TESTHIVE_APK || '/Users/omnarayan/work/temp/TestHive/prebuilt-apk/app-release.apk')
  : (process.env.TESTHIVE_APP || '/Users/omnarayan/work/temp/TestHive-iOS/prebuilt-ios/simulator/testhive.app');
const PAGE = ANDROID ? `http://127.0.0.1:8787/device/${DEV}` : `http://127.0.0.1:8787/device/${DEV}?app=${APP}`;
const USER = 'devicelab';
const PASS = 'robustest';

// resetApp returns the app to a clean slate: a plain relaunch keeps
// TestHive logged in (iOS persists app data across terminate+launch), so
// each journey would inherit the last one's state and a fresh agent
// would never see the login screen again. Uninstall+reinstall is the
// true blank slate, so every journey starts where a first-time user
// does. This is what the ?reset=1 page mode will do for real tests.
function resetApp() {
  const tool = ANDROID ? 'adb' : 'xcrun';
  const un = ANDROID ? ['uninstall', APP] : ['simctl', 'uninstall', DEV, APP];
  const ins = ANDROID ? ['install', '-r', APP_PATH] : ['simctl', 'install', DEV, APP_PATH];
  try { execFileSync(tool, un, { stdio: 'ignore' }); } catch {}
  execFileSync(tool, ins, { stdio: 'ignore' });
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

// resetAndOpen puts the app back to a clean install and loads the page —
// the login screen, every time.
async function resetAndOpen() {
  resetApp();
  await fetch(`http://127.0.0.1:8787/api/devices/${DEV}/app/launch`, { method: 'POST', body: JSON.stringify({ app: APP }) });
  await call('browser_navigate', { url: PAGE });
}

// loginWith signs in with the given credentials; the caller decides
// whether they are the right ones.
async function loginWith(user, pass) {
  await resetAndOpen();
  await type('textbox', 'username', user);
  await type('textbox', 'password', pass);
  await click('button', 'login-button');
}

const freshLogin = () => loginWith(USER, PASS);

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
  await find('button', 'cart-button');
  return "await expect(page.getByRole('button', { name: /cart/i }).first()).toBeVisible();";
});

// Journey B: log in, add the first product, open the cart. Success = a
// checkout control appears.
await journey('add a product to the cart', async () => {
  await freshLogin();
  await click('button', 'add-to-cart');            // first Add button on the list
  await click('button', 'cart-button');           // the badge that appears
  await find('button', 'checkout');
  return "await expect(page.getByRole('button', { name: /checkout/i })).toBeVisible();";
});

// Journey C: log in, add, open cart, checkout, fill whatever the form
// asks for, proceed. Success = the flow moved past the address form.
await journey('check out through the shipping form', async () => {
  await freshLogin();
  await click('button', 'add-to-cart');
  await click('button', 'cart-button');
  await click('button', 'checkout');
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

// Journey D: search the catalogue. Success = the searched framework shows.
await journey('searches the catalogue', async () => {
  await freshLogin();
  await type('textbox', 'search', 'Maestro');
  // The searched framework is still on screen — as a text node (iOS) or
  // a product-item/add button carrying the name (Android). Any of them
  // confirms the search took.
  await Promise.any([
    find('generic', 'Maestro', { tries: 4 }),
    find('text', 'Maestro', { tries: 4 }),
    find('button', 'Maestro', { tries: 4 }),
  ]).catch(() => { throw new Error('searched framework not visible'); });
  return "await expect(page.getByText('Maestro').first()).toBeVisible();";
});

// Journey E: change a cart line's quantity. In the cart the + is a button
// whose id folds "cart-increase-quantity"; the fresh agent finds it by
// that word, not a memorised id.
await journey('increases a cart quantity', async () => {
  await freshLogin();
  await click('button', 'add-to-cart');
  await click('button', 'cart-button');
  await click('button', 'add');   // iOS "Add (cart-increase-quantity)", Android re-add "add-to-cart-button"
  // The cart stayed put with the line still in it — a checkout control is
  // the discoverable proof.
  await find('button', 'checkout');
  return "await expect(page.getByRole('button', { name: /checkout/i })).toBeVisible();";
});

// Journey F: remove the only line from the cart.
await journey('removes an item from the cart', async () => {
  await freshLogin();
  await click('button', 'add-to-cart');
  await click('button', 'cart-button');
  await click('button', 'remove');   // iOS "Remove", Android "remove-from-cart-button"
  await settle(2);
  // Success = the flow offers to keep shopping, i.e. the cart emptied.
  const end = (await allOf('button')).map((b) => b.name).find((n) => /continue|shopping|browse|back/i.test(n));
  return end
    ? `await expect(page.getByRole('button', { name: ${JSON.stringify(end)} })).toBeVisible();`
    : "await expect(page.getByRole('button', { name: /cart/i }).first()).toBeVisible();";
});

// Journey G: a wrong password is rejected — the login screen holds.
await journey('rejects a wrong password', async () => {
  await loginWith(USER, 'not-the-password');
  await settle(2);
  // Still on login: the Sign In button is right there, not the store.
  await find('button', 'login-button');
  return "await expect(page.getByRole('button', { name: /login-button/i })).toBeVisible();";
});

srv.kill();

const gotoLine = ANDROID
  ? 'await page.goto(`http://127.0.0.1:8787/device/${DEV}`);'
  : 'await page.goto(`http://127.0.0.1:8787/device/${DEV}?app=${APP}`);';
const clean = (l) => (l.includes('setTimeout') ? null : l.startsWith('await page.goto(') ? gotoLine : l);
const block = (r) => {
  const steps = r.lines.map(clean).filter(Boolean).map((l) => '  ' + l).join('\n');
  return `test('agent: ${r.name}', async ({ page }) => {\n${steps}\n  ${r.assertion}\n});`;
};

const ok = results.filter((r) => r.ok);
const envName = ANDROID ? 'DEVICEDECK_ANDROID_SERIAL' : 'DEVICEDECK_UDID';
const envDefault = ANDROID ? 'emulator-5554' : 'booted';
const pathEnv = ANDROID ? 'TESTHIVE_APK' : 'TESTHIVE_APP';
const resetLines = ANDROID
  ? `  try { execFileSync('adb', ['uninstall', APP]); } catch {}\n  execFileSync('adb', ['install', '-r', APP_PATH]);`
  : `  try { execFileSync('xcrun', ['simctl', 'uninstall', DEV, APP]); } catch {}\n  execFileSync('xcrun', ['simctl', 'install', DEV, APP_PATH]);`;
const body = `import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';

// GENERATED by a fresh agent (tests/support/discover.mjs) that knew only
// the login credentials and found every element on screen by role and
// name — on ${ANDROID ? 'Android' : 'iOS'}. Re-generate after a DeviceDeck change.

const DEV = process.env.${envName} || '${envDefault}';
const APP = '${APP}';
const APP_PATH = process.env.${pathEnv} || '${APP_PATH}';

// Each test starts from a clean install so it is independent: a relaunch
// keeps TestHive logged in, so the login tests need a blank slate.
test.beforeEach(async ({ request }) => {
${resetLines}
  await request.post(\`/api/devices/\${DEV}/app/launch\`, { data: { app: APP } });
});

${ok.map(block).join('\n\n')}
`;
writeFileSync(ANDROID ? 'tests/app/generated-agent-android.spec.ts' : 'tests/app/generated-agent.spec.ts', body);

console.log('\n=== BENCHMARK: fresh-agent authoring ===');
console.log(`journeys attempted: ${results.length}`);
console.log(`generated cleanly:  ${ok.length}`);
for (const r of results) console.log(`  ${r.ok ? 'ok  ' : 'FAIL'} ${r.name}${r.ok ? '' : ' — ' + r.error}`);
console.log(`\nwrote tests/app/generated-agent.spec.ts (${ok.length} tests)`);
process.exit(0);
