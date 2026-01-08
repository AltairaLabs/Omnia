import { test } from '@playwright/test';

/**
 * Video capture for documentation GIFs.
 *
 * Videos are saved to: e2e/screenshots/videos/
 * Convert to GIFs using: ./scripts/convert-videos.sh
 *
 * Run with: npx playwright test e2e/screenshots/videos.spec.ts --project=video-capture
 */

// Test selectors
const SELECTORS = {
  agentCard: '[data-testid="agent-card"]',
  promptPackCard: '[data-testid="promptpack-card"]',
  toolRegistryCard: '[data-testid="toolregistry-card"]',
  themeToggle: '[data-testid="theme-toggle"]',
} as const;

// Video tests use the video-capture project in playwright.config.ts
test.describe.configure({ mode: 'serial' });

test.describe('Animation Captures', () => {
  test('theme toggle animation', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);
    await page.waitForTimeout(1000);

    // Toggle theme multiple times
    const themeToggle = page.locator(SELECTORS.themeToggle);

    await themeToggle.click();
    await page.waitForTimeout(1000);

    await themeToggle.click();
    await page.waitForTimeout(1000);

    // Video is automatically saved by Playwright
  });

  test('navigation flow', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500);

    // Navigate through main pages
    await page.click('[href="/agents"]');
    await page.waitForSelector(SELECTORS.agentCard);
    await page.waitForTimeout(800);

    await page.click('[href="/promptpacks"]');
    await page.waitForSelector(SELECTORS.promptPackCard);
    await page.waitForTimeout(800);

    await page.click('[href="/tools"]');
    await page.waitForSelector(SELECTORS.toolRegistryCard);
    await page.waitForTimeout(800);

    await page.click('[href="/"]');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(500);
  });

  test('agent detail tabs', async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    await page.locator(SELECTORS.agentCard).first().click();
    await page.waitForURL(/agents\/[^/]+/);
    await page.waitForTimeout(500);

    // Click through each tab
    const tabs = ['Overview', 'Console', 'Logs', 'Events'];
    for (const tabName of tabs) {
      const tab = page.locator(`[role="tab"]:has-text("${tabName}")`);
      if (await tab.isVisible()) {
        await tab.click();
        await page.waitForTimeout(800);
      }
    }
  });
});
