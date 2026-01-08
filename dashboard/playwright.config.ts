import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for Omnia Dashboard.
 *
 * Projects:
 * - chromium: E2E tests (npm run test:e2e)
 * - screenshots: Screenshot capture (npm run screenshots)
 * - video-capture: Video capture for GIFs (npm run videos)
 *
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e',
  /* Fail the build on CI if you accidentally left test.only in the source code */
  forbidOnly: !!process.env.CI,
  /* Retry failed tests on CI */
  retries: process.env.CI ? 2 : 0,
  /* Limit parallel workers on CI to avoid resource issues */
  workers: process.env.CI ? 1 : undefined,
  /* Reporter to use */
  reporter: process.env.CI
    ? [['junit', { outputFile: 'e2e-results.xml' }], ['html', { open: 'never' }]]
    : [['html', { open: 'on-failure' }]],
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
      testIgnore: '**/screenshots/**',
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
