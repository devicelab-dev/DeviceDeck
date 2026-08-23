import type { Reporter, TestCase, TestResult } from '@playwright/test/reporter';
import { execFileSync } from 'child_process';
import { mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

// Attaches the real device screen to any failed test, for EVERY test
// regardless of how it imports `test` — no fixture, no per-test change.
//
// It works because the capture is synchronous: onTestEnd runs between
// tests (workers=1), the failed device state is still on screen, and a
// blocking simctl grab completes before the next test's reset. An async
// fetch here would not — Playwright does not await async reporter hooks —
// which is why the fixture, not a reporter, was needed until this.
export default class DeviceScreenshotReporter implements Reporter {
  private base = process.env.DEVICEDECK_URL || 'http://127.0.0.1:8787';
  // Android tests set the serial; iOS tests the udid. The server drives
  // both by the same id, so either identifies the device to screenshot.
  private udid = process.env.DEVICEDECK_ANDROID_SERIAL || process.env.DEVICEDECK_UDID || 'booted';

  onTestEnd(_test: TestCase, result: TestResult) {
    if (result.status === 'passed' || result.status === 'skipped') return;
    try {
      const path = join(mkdtempSync(join(tmpdir(), 'dd-shot-')), 'device-screen.png');
      // Through DeviceDeck's own endpoint so it works for any device the
      // server drives — iOS simctl or Android adb — not just simulators.
      // Synchronous curl: onTestEnd is not awaited, but a blocking call
      // finishes before Playwright moves to the next test.
      execFileSync('curl', ['-s', '-f', '-m', '10', '-o', path,
        `${this.base}/api/devices/${this.udid}/screenshot`], { stdio: 'ignore' });
      result.attachments.push({ name: 'device-screen-at-failure', path, contentType: 'image/png' });
    } catch {
      // The device being uncapturable is already the failure under report.
    }
  }
}
