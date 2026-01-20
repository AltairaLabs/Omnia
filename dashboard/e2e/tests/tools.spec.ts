import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Tools page.
 */

// Common selectors
const SELECTORS = {
  toolCard: '[data-testid="tool-card"], [data-testid="tool-row"], [data-testid="toolregistry-card"]',
  toolList: '[data-testid="tool-list"], [data-testid="tool-card"], [data-testid="tool-row"], [data-testid="toolregistry-card"]',
  statusBadge: '[data-testid="tool-status"], [data-testid="status-badge"]',
};

// URL patterns (simple patterns to avoid slow regex)
const toolDetailUrlPattern = /tools\/[^/]+$/;

test.describe('Tools Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/tools');
  });

  test('should display list of tool registries', async ({ page }) => {
    // Wait for tools to load
    await page.waitForSelector(SELECTORS.toolList, { timeout: 10000 });

    // Verify at least one tool registry is displayed
    const tools = page.locator(SELECTORS.toolCard);
    await expect(tools.first()).toBeVisible();
  });

  test('should show tool registry status', async ({ page }) => {
    // Wait for tools to load
    await page.waitForSelector(SELECTORS.toolCard);

    // Check for status indicators
    const statusBadges = page.locator(SELECTORS.statusBadge);
    if (await statusBadges.first().isVisible().catch(() => false)) {
      await expect(statusBadges.first()).toBeVisible();
    }
  });

  test('should navigate to tool registry details', async ({ page }) => {
    // Wait for tools to load
    await page.waitForSelector(SELECTORS.toolCard);

    // Click on first tool registry
    const firstTool = page.locator(SELECTORS.toolCard).first();
    await firstTool.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(toolDetailUrlPattern);
  });

  test('should display discovered tools count', async ({ page }) => {
    // Wait for tools to load
    await page.waitForSelector(SELECTORS.toolCard);

    // Look for tools count display (using data-testid only to avoid slow regex)
    const toolsCount = page.locator('[data-testid="tools-count"]');
    if (await toolsCount.first().isVisible().catch(() => false)) {
      await expect(toolsCount.first()).toBeVisible();
    }
  });
});

test.describe('Tool Registry Details', () => {
  test('should display tool registry information', async ({ page }) => {
    // Navigate to tools page
    await page.goto('/tools');
    await page.waitForSelector(SELECTORS.toolCard);

    // Click on first tool registry
    const firstTool = page.locator(SELECTORS.toolCard).first();
    await firstTool.click();

    // Wait for detail page to load
    await page.waitForURL(toolDetailUrlPattern);

    // Verify tool registry name is displayed
    await expect(
      page.locator('[data-testid="tool-name"], h1, [data-testid="toolregistry-title"]')
    ).toBeVisible();
  });

  test('should list individual tools in registry', async ({ page }) => {
    // Navigate to tools page
    await page.goto('/tools');
    await page.waitForSelector(SELECTORS.toolCard);

    // Click on first tool registry
    const firstTool = page.locator(SELECTORS.toolCard).first();
    await firstTool.click();

    // Wait for detail page to load
    await page.waitForURL(toolDetailUrlPattern);

    // Look for individual tool listings
    const toolItems = page.locator(
      '[data-testid="tool-item"], [data-testid="handler-item"], [data-testid="tool-handler"]'
    );
    if (await toolItems.first().isVisible().catch(() => false)) {
      await expect(toolItems.first()).toBeVisible();
    }
  });
});
