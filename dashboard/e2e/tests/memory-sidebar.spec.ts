import { test, expect } from '../fixtures/coverage';
import type { Page } from '@playwright/test';

// Selector for session rows across different table/card layouts
const SESSION_ROW_SELECTOR = '[data-testid="session-row"], [data-testid="session-card"], table tbody tr';
const MEMORIES_TOGGLE = '[data-testid="memories-toggle"]';

async function navigateToFirstSession(page: Page): Promise<boolean> {
  await page.goto('/sessions');
  await page.waitForSelector(SESSION_ROW_SELECTOR, { timeout: 10000 });
  const firstSession = page.locator(SESSION_ROW_SELECTOR).first();
  if (!(await firstSession.isVisible({ timeout: 5000 }).catch(() => false))) {
    return false;
  }
  await firstSession.click();
  return true;
}

test.describe('Memory Sidebar', () => {
  test('should show memories button on session page', async ({ page }) => {
    const navigated = await navigateToFirstSession(page);
    if (navigated) {
      await expect(page.locator(MEMORIES_TOGGLE)).toBeVisible({ timeout: 5000 });
    }
  });

  test('should open sidebar on click', async ({ page }) => {
    const navigated = await navigateToFirstSession(page);
    if (navigated) {
      await page.locator(MEMORIES_TOGGLE).click();
      await expect(page.locator('[data-testid="memory-sidebar"]')).toBeVisible({ timeout: 5000 });
    }
  });

  test('should show view all link in sidebar', async ({ page }) => {
    const navigated = await navigateToFirstSession(page);
    if (navigated) {
      await page.locator(MEMORIES_TOGGLE).click();
      await expect(page.locator('[data-testid="view-all-memories"]')).toBeVisible({ timeout: 5000 });
    }
  });
});
