// The device is a normal webpage: Puppeteer drives the simulator through
// the DOM mirror with its standard page API. Requires `devicedeck serve`
// and a booted simulator with TestHive installed (freshly launched).

const { test, before, after } = require("node:test");
const assert = require("node:assert");
const puppeteer = require("puppeteer");

const BASE = process.env.DEVICEDECK_URL || "http://127.0.0.1:8787";
const UDID = process.env.DEVICEDECK_UDID || "booted";
const APP = "dev.devicelab.testhive";

let browser;

before(async () => {
  browser = await puppeteer.launch();
});

after(async () => {
  await browser.close();
});

test("logs into TestHive on a real simulator", async () => {
  const page = await browser.newPage();
  await page.goto(`${BASE}/device/${UDID}?app=${APP}`);

  // First mirror render needs the tree engine warm; give it time.
  const username = await page.waitForSelector(
    '[data-testid="username-input"]',
    { visible: true, timeout: 30_000 },
  );

  await username.click();
  // Keys forward as real HID presses (~100ms hold each) — pace the typing.
  await page.keyboard.type("devicelab", { delay: 150 });

  await page.click('[data-testid="password-input"]');
  await page.keyboard.type("robustest", { delay: 150 });

  await page.click('[data-testid="login-button"]');

  // Post-login home screen: mirror text comes from native labels.
  const greeting = await page.waitForSelector(
    "text/Hello, devicelab!",
    { timeout: 20_000 },
  );
  assert.ok(greeting, "expected post-login greeting in the mirror");
});
