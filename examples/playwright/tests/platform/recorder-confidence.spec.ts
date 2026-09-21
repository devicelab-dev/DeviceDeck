import { test, expect, type Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

// The recorder colours each captured step by how durable its selector is —
// green id, yellow text, orange relational, red index or coordinate — so a
// reviewer sees a brittle step live while capturing, not only in the
// exported flow's comments. Driven against the real console page with the
// device and capture endpoints stubbed; no simulator needed.

const STATIC = path.join(__dirname, '../../../../internal/web/static');
const UDID = '00000000-0000-0000-0000-000000000000';
const TYPES: Record<string, string> = {
  '.js': 'application/javascript', '.html': 'text/html', '.css': 'text/css', '.svg': 'image/svg+xml',
};

// A step with a resolved bound at the screen centre, varying only in how it
// is addressed.
const bounds = { x: 0.4, y: 0.4, width: 0.2, height: 0.1 };
const step = (over: Record<string, unknown>) => ({ kind: 'tapOn', bounds, ...over });

async function openConsole(page: Page, steps: unknown[]) {
  await page.routeWebSocket(/\/(video|input)$/, () => {});
  await page.route('**/api/devices', (r) =>
    r.fulfill({ json: { devices: [{ udid: UDID, name: 'Fixture', booted: true, os: 'iOS 26.2' }] } }));
  await page.route('**/api/devices/*/capture/start', (r) => r.fulfill({ json: { ok: true } }));
  // The capture poll returns the growing step list; each test sets it.
  await page.route('**/api/devices/*/capture', (r) =>
    r.fulfill({ json: { recording: true, steps } }));
  await page.route(/\/(index\.html)?(\?.*)?$/, (r) =>
    r.fulfill({ contentType: 'text/html', body: fs.readFileSync(path.join(STATIC, 'index.html'), 'utf8') }));
  await page.route(/\.(js|css|svg)$/, (r) => {
    const file = path.join(STATIC, path.basename(new URL(r.request().url()).pathname));
    if (!fs.existsSync(file)) return r.fulfill({ status: 404, body: '' });
    return r.fulfill({ contentType: TYPES[path.extname(file)], body: fs.readFileSync(file, 'utf8') });
  });
  await page.goto(`http://127.0.0.1:8787/?device=${UDID}`);
  await expect(page.locator('#btn-record')).toBeVisible({ timeout: 15_000 });
}

async function gradeOf(page: Page, s: Record<string, unknown>): Promise<string> {
  await openConsole(page, [step(s)]);
  // Recording needs an app bundle id; capture/start is stubbed to accept it.
  await page.locator('#app').fill('dev.devicelab.testhive');
  await page.getByRole('button', { name: /Record/ }).click();
  const flash = page.locator('#overlay .resolved-flash');
  await expect(flash).toBeVisible({ timeout: 5_000 });
  const cls = (await flash.getAttribute('class')) || '';
  return (cls.match(/grade-\w+/) || [''])[0];
}

test('an identifier grades green (id)', async ({ page }) => {
  expect(await gradeOf(page, { id: 'login-button' })).toBe('grade-id');
  await expect(page.locator('#overlay .resolved-flash span')).toHaveText('id: login-button · id');
});

test('a text match grades yellow', async ({ page }) => {
  expect(await gradeOf(page, { text: 'Continue' })).toBe('grade-text');
});

test('a relational qualifier grades orange', async ({ page }) => {
  expect(await gradeOf(page, { id: 'row-cta', childOfId: 'alice-row' })).toBe('grade-relational');
});

test('a positional index grades red', async ({ page }) => {
  expect(await gradeOf(page, { id: 'cell', index: 2 })).toBe('grade-index');
});
