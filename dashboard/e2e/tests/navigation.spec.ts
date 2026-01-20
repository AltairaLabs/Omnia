import { test, expect } from '../fixtures/coverage';

/**
 * Basic navigation tests for the Omnia Dashboard.
 * These tests verify that all main pages are accessible and render correctly.
 *
 * Uses coverage fixture for E2E code coverage collection.
 */

// URL patterns (simple patterns to avoid slow regex)
const urlPatterns = {
  agents: /agents$/,
  promptPacks: /promptpacks$/,
  tools: /tools$/,
};

test.describe('Navigation', () => {
  test.beforeEach(async ({ page }) => {
    // Start at the home page
    await page.goto('/');
  });

  test('should load the dashboard home page', async ({ page }) => {
    // Wait for the page to load
    await expect(page).toHaveTitle(/Omnia/i);
  });

  test('should navigate to agents page', async ({ page }) => {
    // Click on agents link in sidebar or nav
    await page.click('[data-testid="nav-agents"], [href="/agents"]');
    await expect(page).toHaveURL(urlPatterns.agents);

    // Verify agents page content
    await expect(page.locator('h1, [data-testid="agents-title"]')).toContainText(/Agents/i);
  });

  test('should navigate to prompt packs page', async ({ page }) => {
    await page.click('[data-testid="nav-promptpacks"], [href="/promptpacks"]');
    await expect(page).toHaveURL(urlPatterns.promptPacks);
  });

  test('should navigate to tools page', async ({ page }) => {
    await page.click('[data-testid="nav-tools"], [href="/tools"]');
    await expect(page).toHaveURL(urlPatterns.tools);
  });

  test('should toggle theme between light and dark', async ({ page }) => {
    // Find and click theme toggle
    const themeToggle = page.locator('[data-testid="theme-toggle"]');

    // Get initial theme
    const html = page.locator('html');
    const initialClass = await html.getAttribute('class');
    const initialDark = initialClass?.includes('dark');

    // Toggle theme
    await themeToggle.click();

    // Verify theme changed
    const newClass = await html.getAttribute('class');
    const newDark = newClass?.includes('dark');
    expect(newDark).not.toBe(initialDark);
  });

  test('should show sidebar navigation items', async ({ page }) => {
    // Verify sidebar has expected navigation items
    const sidebar = page.locator('[data-testid="sidebar"], nav');

    await expect(sidebar.locator('text=Agents')).toBeVisible();
    await expect(sidebar.locator('text=PromptPacks')).toBeVisible();
    await expect(sidebar.locator('text=Tools')).toBeVisible();
  });
});

test.describe('Responsive Navigation', () => {
  test('should show mobile menu on small screens', async ({ page }) => {
    // Set viewport to mobile size
    await page.setViewportSize({ width: 375, height: 667 });
    await page.goto('/');

    // Mobile menu button should be visible
    const menuButton = page.locator('[data-testid="mobile-menu-toggle"]');

    // If mobile menu exists, test it
    if (await menuButton.isVisible()) {
      await menuButton.click();

      // Verify mobile navigation appears
      await expect(page.locator('[data-testid="mobile-nav"]')).toBeVisible();
    }
  });
});
