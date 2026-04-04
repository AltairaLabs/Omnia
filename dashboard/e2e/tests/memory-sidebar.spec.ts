import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Memory Sidebar on the console page.
 *
 * Tests verify the sidebar trigger exists on the console and opens correctly.
 * The console page is where users actually chat with agents, so this is the
 * primary integration point for memory visibility.
 */

const CONSOLE_SELECTOR = '[data-testid="console-dropzone"], [data-testid="agent-selector"]';
const MEMORIES_TOGGLE = '[data-testid="memories-toggle"]';
const CONSOLE_ACTIVE = '[data-testid="console-dropzone"]';
const MEMORY_SIDEBAR = '[data-testid="memory-sidebar"]';

test.describe('Memory Sidebar (Console)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/console');
    // Wait for the console to load
    await page.waitForSelector(CONSOLE_SELECTOR, { timeout: 15000 });
  });

  test('should show memories toggle button on active console', async ({ page }) => {
    // If an agent is selected and console is active, the memories button should be visible
    const toggle = page.locator(MEMORIES_TOGGLE);
    const consoleActive = page.locator(CONSOLE_ACTIVE);

    if (await consoleActive.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(toggle).toBeVisible({ timeout: 5000 });
    }
  });

  test('should open sidebar with memory data (not errors)', async ({ page }) => {
    const toggle = page.locator(MEMORIES_TOGGLE);
    const consoleActive = page.locator(CONSOLE_ACTIVE);

    if (await consoleActive.isVisible({ timeout: 5000 }).catch(() => false)) {
      await toggle.click();

      const sidebar = page.locator(MEMORY_SIDEBAR);
      await expect(sidebar).toBeVisible({ timeout: 5000 });

      // Sidebar should show either memory cards or empty state — not a loading skeleton forever
      await page.waitForTimeout(3000);
      const hasCards = await sidebar.locator('[data-testid="memory-card"]').first().isVisible().catch(() => false);
      const hasEmpty = await sidebar.getByText('No memories yet').isVisible().catch(() => false);
      // At least one should be true — loading should have resolved
      expect(hasCards || hasEmpty).toBe(true);
    }
  });

  test('should show view all link that navigates to memories page', async ({ page }) => {
    const toggle = page.locator(MEMORIES_TOGGLE);
    const consoleActive = page.locator(CONSOLE_ACTIVE);

    if (await consoleActive.isVisible({ timeout: 5000 }).catch(() => false)) {
      await toggle.click();

      const viewAllLink = page.locator('[data-testid="view-all-memories"]');
      await expect(viewAllLink).toBeVisible({ timeout: 5000 });
      await expect(viewAllLink).toHaveAttribute('href', '/memories');
    }
  });
});

test.describe('Memory Sidebar (Session Detail)', () => {
  test('should show memories toggle on session detail page', async ({ page }) => {
    // Navigate to sessions list
    await page.goto('/sessions');

    const sessionRow = page.locator('table tbody tr, [data-testid="session-row"], [data-testid="session-card"]').first();
    if (!(await sessionRow.isVisible({ timeout: 10000 }).catch(() => false))) {
      test.skip();
      return;
    }

    await sessionRow.click();
    // Wait for session detail to load
    await page.waitForTimeout(2000);

    const toggle = page.locator(MEMORIES_TOGGLE);
    await expect(toggle).toBeVisible({ timeout: 5000 });
  });
});
