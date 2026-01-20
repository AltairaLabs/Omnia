import { test as base, expect, type Page, type TestInfo } from '@playwright/test';
// Use addCoverageReport from monocart-reporter for proper coverage collection
import { addCoverageReport } from 'monocart-reporter';

/**
 * Coverage fixture for E2E tests.
 *
 * Collects V8 JavaScript coverage during test execution.
 * Enable with: COLLECT_COVERAGE=true npm run test:e2e
 *
 * The coverage data is collected by monocart-reporter and merged
 * with unit test coverage for SonarCloud reporting.
 *
 * Usage: Import `test` from this file instead of '@playwright/test'
 *
 * @example
 * import { test, expect } from '../fixtures/coverage';
 */

const collectCoverage = process.env.COLLECT_COVERAGE === 'true';

type CoverageFixtures = {
  /** Coverage collection is automatic when COLLECT_COVERAGE=true */
  autoCollectCoverage: void;
};

// Extend base test with coverage collection
export const test = base.extend<CoverageFixtures>({
  // Auto-use fixture that manages coverage per test
  autoCollectCoverage: [async ({ page }: { page: Page }, use: () => Promise<void>, testInfo: TestInfo) => {
    // Only collect coverage in Chromium (V8 coverage is Chromium-only)
    const isChromium = testInfo.project.name === 'chromium';

    if (collectCoverage && isChromium) {
      // Start collecting JS coverage
      await page.coverage.startJSCoverage({
        resetOnNavigation: false,
      });
    }

    // Run the test
    await use();

    if (collectCoverage && isChromium) {
      // Stop coverage and get the data
      const coverageData = await page.coverage.stopJSCoverage();

      // Add coverage to monocart-reporter
      if (coverageData.length > 0) {
        await addCoverageReport(coverageData, testInfo);
      }
    }
  }, { auto: true }],
});

export { expect };

/**
 * Re-export test with coverage for explicit imports.
 */
export const testWithCoverage = test;
