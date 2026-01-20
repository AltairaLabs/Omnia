import { defineConfig, devices, type ReporterDescription } from '@playwright/test';

/**
 * Playwright configuration for Omnia Dashboard.
 *
 * Projects:
 * - chromium: E2E tests (npm run test:e2e)
 * - screenshots: Screenshot capture (npm run screenshots)
 * - video-capture: Video capture for GIFs (npm run videos)
 *
 * Coverage:
 * - Uses monocart-reporter to collect V8 coverage during E2E tests
 * - Outputs lcov format for SonarCloud integration
 * - Enable with: COLLECT_COVERAGE=true npm run test:e2e
 *
 * See https://playwright.dev/docs/test-configuration.
 */

const collectCoverage = process.env.COLLECT_COVERAGE === 'true';
const isCI = !!process.env.CI;

/**
 * Get the reporter configuration based on environment.
 */
function getReporterConfig(): ReporterDescription[] {
  if (collectCoverage) {
    return [
      ['monocart-reporter', {
        name: 'Omnia Dashboard E2E Coverage Report',
        outputFile: './e2e-coverage/report.html',
        coverage: {
          reportPath: './e2e-coverage/coverage',
          reports: ['v8', 'lcovonly', 'console-summary'],
          lcov: {
            outputFile: './e2e-coverage/lcov.info',
          },
          entryFilter: (entry: { url: string }) => {
            return entry.url.includes('localhost:3000') || entry.url.includes('127.0.0.1:3000');
          },
          sourceFilter: (sourcePath: string) => {
            if (sourcePath.includes('node_modules')) return false;
            if (sourcePath.includes('_next/static')) return false;
            if (sourcePath.includes('.next')) return false;
            return sourcePath.includes('/src/');
          },
        },
      }],
      ['html', { open: 'never' }],
    ];
  }

  if (isCI) {
    return [['junit', { outputFile: 'e2e-results.xml' }], ['html', { open: 'never' }]];
  }

  return [['html', { open: 'on-failure' }]];
}

export default defineConfig({
  testDir: './e2e',
  /* Fail the build on CI if you accidentally left test.only in the source code */
  forbidOnly: isCI,
  /* Retry failed tests on CI */
  retries: isCI ? 2 : 0,
  /* Limit parallel workers on CI to avoid resource issues */
  workers: isCI ? 1 : undefined,
  /* Reporter to use */
  reporter: getReporterConfig(),
  /* Shared settings for all projects */
  use: {
    /* Base URL for the app */
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000',
    /* Collect trace when retrying a failed test */
    trace: 'on-first-retry',
  },

  /* Configure projects */
  projects: [
    /* E2E tests - run with: npm run test:e2e */
    {
      name: 'chromium',
      testIgnore: ['**/screenshots/**', '**/multimodal/**'],
      use: {
        ...devices['Desktop Chrome'],
        screenshot: 'only-on-failure',
        video: 'on-first-retry',
      },
    },
    /* Screenshot capture - run with: npm run screenshots */
    {
      name: 'screenshots',
      testMatch: '**/screenshots/capture.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        screenshot: 'off',
        video: 'off',
      },
    },
    /* Video capture for GIFs - run with: npm run videos */
    {
      name: 'video-capture',
      testMatch: '**/screenshots/videos.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 800, height: 600 },
        video: {
          mode: 'on',
          size: { width: 800, height: 600 },
        },
      },
    },
  ],

  /* Run local dev server before tests if not in CI */
  webServer: process.env.CI
    ? undefined
    : {
        command: 'npm run dev',
        url: 'http://localhost:3000',
        reuseExistingServer: !process.env.CI,
        timeout: 120 * 1000,
        env: {
          // Enable mock mode for consistent test data
          DEMO_MODE: 'true',
          NEXT_PUBLIC_DEMO_MODE: 'true',
        },
      },

  /* Output directory for test artifacts */
  outputDir: 'test-results/',
});
