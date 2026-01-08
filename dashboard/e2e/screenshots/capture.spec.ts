import { test } from '@playwright/test';

/**
 * Automated screenshot capture for documentation.
 *
 * This spec captures high-quality screenshots of the dashboard
 * for use in README, docs, and marketing materials.
 *
 * Screenshots are saved to: docs/src/assets/screenshots/
 *
 * Run with: npm run screenshots
 */

// Output directory
const SCREENSHOT_DIR = '../docs/src/assets/screenshots';

// Common viewport sizes
const DESKTOP_VIEWPORT = { width: 1280, height: 720 };
const WIDE_VIEWPORT = { width: 1440, height: 900 };

// Test selectors
const SELECTORS = {
  agentCard: '[data-testid="agent-card"]',
  promptPackCard: '[data-testid="promptpack-card"]',
  toolRegistryCard: '[data-testid="toolregistry-card"]',
  themeToggle: '[data-testid="theme-toggle"]',
} as const;

// Set viewport for all tests in this file
test.use({
  viewport: DESKTOP_VIEWPORT,
});

test.describe('Dashboard Screenshots', () => {
  test.beforeEach(async ({ page }) => {
    // Wait for fonts and styles to load
    await page.goto('/');
    await page.waitForLoadState('networkidle');
  });

  test('dashboard overview', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500); // Allow animations to settle

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/dashboard-overview.png`,
      fullPage: false,
    });
  });

  test('agents list - card view', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard, { timeout: 10000 });
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/agents-list.png`,
      fullPage: false,
    });
  });

  test('agent detail page', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    // Click first agent to go to detail
    await page.locator(SELECTORS.agentCard).first().click();
    await page.waitForURL(/agents\/[^/]+/);
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/agent-detail.png`,
      fullPage: false,
    });
  });

  test('agent console tab', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);
    await page.locator(SELECTORS.agentCard).first().click();
    await page.waitForURL(/agents\/[^/]+/);

    // Click Console tab
    const consoleTab = page.locator('[role="tab"]:has-text("Console")');
    if (await consoleTab.isVisible()) {
      await consoleTab.click();
      await page.waitForTimeout(500);

      await page.screenshot({
        path: `${SCREENSHOT_DIR}/agent-console.png`,
        fullPage: false,
      });
    }
  });

  test('promptpacks list', async ({ page }) => {
    await page.goto('/promptpacks');
    await page.waitForSelector(SELECTORS.promptPackCard, { timeout: 10000 });
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/promptpacks-list.png`,
      fullPage: false,
    });
  });

  test('tools list', async ({ page }) => {
    await page.goto('/tools');
    await page.waitForSelector(SELECTORS.toolRegistryCard, { timeout: 10000 });
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/tools-list.png`,
      fullPage: false,
    });
  });

  test('topology view', async ({ page }) => {
    await page.goto('/topology');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000); // Graph needs time to render

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/topology.png`,
      fullPage: false,
    });
  });

  test('costs page', async ({ page }) => {
    await page.goto('/costs');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/costs.png`,
      fullPage: false,
    });
  });

  test('dark mode - dashboard', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // Toggle to dark mode
    const themeToggle = page.locator(SELECTORS.themeToggle);
    await themeToggle.click();
    await page.waitForTimeout(300); // Wait for theme transition

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/dashboard-dark.png`,
      fullPage: false,
    });
  });

  test('dark mode - agents', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    // Toggle to dark mode
    const themeToggle = page.locator(SELECTORS.themeToggle);
    await themeToggle.click();
    await page.waitForTimeout(300);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/agents-dark.png`,
      fullPage: false,
    });
  });
});

test.describe('Hero Screenshots (Wide)', () => {
  test.use({
    viewport: WIDE_VIEWPORT,
  });

  test('hero - dashboard wide', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/hero-dashboard.png`,
      fullPage: false,
    });
  });

  test('hero - agents wide', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);
    await page.waitForTimeout(500);

    await page.screenshot({
      path: `${SCREENSHOT_DIR}/hero-agents.png`,
      fullPage: false,
    });
  });
});
