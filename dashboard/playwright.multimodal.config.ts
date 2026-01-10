import { defineConfig, devices } from '@playwright/test';
import path from 'path';

/**
 * Playwright configuration for multi-modal console E2E tests.
 *
 * This config runs the dashboard with a real agent binary (using demo handler mode)
 * to test multi-modal features like images, audio, video, and file uploads.
 *
 * Run with: npm run test:e2e:multimodal
 *
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e/tests/multimodal',
  /* Fail the build on CI if you accidentally left test.only in the source code */
  forbidOnly: !!process.env.CI,
  /* Retry failed tests on CI */
  retries: process.env.CI ? 2 : 0,
  /* Single worker to avoid WebSocket connection conflicts with the test agent */
  workers: 1,
  /* Longer timeout for multi-modal tests that involve streaming */
  timeout: 60000,
  /* Reporter to use */
  reporter: process.env.CI
    ? [['junit', { outputFile: 'e2e-multimodal-results.xml' }], ['html', { open: 'never' }]]
    : [['html', { open: 'on-failure' }]],
  /* Shared settings for all projects */
  use: {
    /* Base URL for the app */
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000',
    /* Collect trace when retrying a failed test */
    trace: 'on-first-retry',
    /* Capture screenshots on failure */
    screenshot: 'only-on-failure',
    /* Capture video on first retry */
    video: 'on-first-retry',
  },

  /* Configure projects */
  projects: [
    {
      name: 'multimodal',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],

  /* Run both dashboard and agent servers before tests */
  webServer: [
    {
      // Dashboard server - uses demo mode for mock data but real WebSocket for console
      command: 'npm run dev',
      url: 'http://localhost:3000',
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
      env: {
        // Keep demo mode enabled for mock agent data
        DEMO_MODE: 'true',
        NEXT_PUBLIC_DEMO_MODE: 'true',
        // Point to local agent facade - this enables real WebSocket in MockDataService
        NEXT_PUBLIC_WS_PROXY_URL: 'ws://localhost:8080',
        // Direct mode: connect to agent's /ws endpoint directly (not through proxy)
        NEXT_PUBLIC_WS_DIRECT_MODE: 'true',
      },
    },
    {
      // Agent server with demo handler (returns canned multi-modal responses)
      // In CI, use pre-built binary; locally use go run
      command: process.env.CI
        ? `${path.resolve(__dirname, '../bin/agent')}`
        : `go run ${path.resolve(__dirname, '../cmd/agent/main.go')}`,
      url: 'http://localhost:8081/healthz',
      reuseExistingServer: !process.env.CI,
      timeout: 60 * 1000,
      env: {
        OMNIA_HANDLER_MODE: 'demo',
        OMNIA_AGENT_NAME: 'e2e-test-agent',
        OMNIA_NAMESPACE: 'default',
        OMNIA_PROMPTPACK_NAME: 'e2e-test-promptpack',
        OMNIA_FACADE_PORT: '8080',
        OMNIA_HEALTH_PORT: '8081',
        OMNIA_SESSION_TYPE: 'memory',
      },
    },
  ],

  /* Output directory for test artifacts */
  outputDir: 'test-results/multimodal/',
});
