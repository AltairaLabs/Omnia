import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for Omnia Dashboard E2E tests.
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e',
  /* Run tests in parallel */
  fullyParallel: true,
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
    /* Capture screenshot on failure */
    screenshot: 'only-on-failure',
    /* Video recording on failure */
    video: 'on-first-retry',
  },

  /* Configure projects for major browsers */
  projects: [
    /* Desktop browsers */
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    /* Uncomment to enable more browsers */
    // {
    //   name: 'firefox',
    //   use: { ...devices['Desktop Firefox'] },
    // },
    // {
    //   name: 'webkit',
    //   use: { ...devices['Desktop Safari'] },
    // },
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
