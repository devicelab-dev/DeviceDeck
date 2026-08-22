import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

// A native accessibility tree aggregates labels upward, so one button's
// label also names every wrapper above it. An agent reads the mirror as
// an AI-mode snapshot, where that arrives as a column of identical
// entries with identical boxes and no way to tell which one to click —
// the 2026-08-19 capture of TestHive's login screen spent eleven lines
// on it before reaching the single button that existed.
//
// This spec is the regression guard for dropRepeatedName. It needs no
// device: the wrapper chain is built to the exact shape syncNode emits,
// and the real function is lifted out of device.js and run against it.
// The device-backed specs prove the mirror matches a live app; this one
// proves what an agent reads of it.

const ROOT_NAME = 'device screen ab12cd34';
const CHAIN = ['Typing Predictions', 'Typing Predictions', 'Typing Predictions',
               'Passwords', 'Passwords'];

// dropRepeatedName's source, lifted from the page script. device.js is
// a classic script rather than a module — it runs start() on load and
// opens sockets — so it cannot be imported, and copying the function
// here would let the copy drift from the one that ships.
function dropRepeatedNameSource(): string {
  const src = fs.readFileSync(
    path.join(__dirname, '../../../../internal/web/static/device.js'), 'utf8');
  const from = src.indexOf('function dropRepeatedName');
  expect(from, 'dropRepeatedName still exists in device.js').toBeGreaterThan(-1);
  const fn = src.slice(from);
  return fn.slice(0, fn.indexOf('\n}\n') + 2);
}

// Text nodes are built by script, not markup: indentation in markup
// would leave whitespace inside them, so they would no longer be the
// exact duplicates the function looks for and the test would pass while
// the real page still repeated itself.
async function buildChain(page: any) {
  await page.setContent(`<div id="m" aria-label="${ROOT_NAME}"></div>`);
  await page.evaluate(([chain]: [string[]]) => {
    let parent = document.getElementById('m')!;
    for (const label of chain) {
      const el = document.createElement('div');
      el.setAttribute('aria-label', label);
      el.appendChild(document.createTextNode(label));
      parent.appendChild(el);
      parent = el;
    }
    const btn = document.createElement('div');
    btn.setAttribute('role', 'button');
    btn.setAttribute('aria-label', 'Passwords');
    btn.appendChild(document.createTextNode('Passwords'));
    parent.appendChild(btn);
  }, [CHAIN]);
}

test('an echoing wrapper chain collapses to the control', async ({ page }) => {
  await buildChain(page);
  const snap = () => page.locator('#m').ariaSnapshot({ mode: 'ai' });

  const before = await snap();
  expect(before.match(/generic "Passwords"/g)).toHaveLength(2);

  await page.evaluate(([code]: [string]) => {
    const drop = eval(`(${code})`) as (el: Element, name: string | null) => void;
    for (const el of document.querySelectorAll('#m [aria-label]')) {
      const parent = el.parentElement!;
      if (parent.id !== 'm') drop(parent, el.getAttribute('aria-label'));
    }
  }, [dropRepeatedNameSource()]);

  const after = await snap();
  // The control is the only thing left carrying the name, and it is
  // still addressable by it.
  expect(after).not.toMatch(/generic "Passwords"/);
  expect(after).toMatch(/button "Passwords"/);
  await expect(page.getByRole('button', { name: 'Passwords' })).toHaveCount(1);
  // getByText has to keep working: a stripped text node was only ever a
  // duplicate of one further down.
  await expect(page.getByText('Passwords', { exact: true }).last()).toBeAttached();
  // The innermost wrapper of a chain keeps its name — nothing beneath it
  // repeats it, so it is the node that earned it.
  expect(after).toMatch(/generic "Typing Predictions"/);
  // The root's name is the screen fingerprint and belongs to no node.
  expect(after).toContain(ROOT_NAME);

  expect(after.split('\n').length).toBeLessThan(before.split('\n').length / 2);
});
