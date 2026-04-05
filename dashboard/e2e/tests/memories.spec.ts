import { test, expect } from '../fixtures/coverage';

const ANONYMOUS_NOTICE = '[data-testid="memory-anonymous-notice"]';

/**
 * E2E tests for the Memories page.
 *
 * The E2E suite runs with OMNIA_AUTH_ANONYMOUS_ROLE=editor, so the session is
 * anonymous. Memory operations require an authenticated user_id, so the page
 * surfaces a sign-in notice, hides the toolbar, and skips the memory-api
 * fetch entirely. These tests verify that anonymous UX.
 *
 * Proxy-route wiring for authenticated users is covered by the unit tests
 * under src/app/api/workspaces/[name]/memory/.
 */
test.describe('Memories Page (anonymous)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/memories');
    // Anonymous notice is rendered synchronously once the auth context resolves
    await page.waitForSelector(ANONYMOUS_NOTICE, { timeout: 10000 });
  });

  test('should show the sign-in notice', async ({ page }) => {
    const notice = page.locator(ANONYMOUS_NOTICE);
    await expect(notice).toBeVisible();
    await expect(notice).toContainText(/sign in/i);
  });

  test('should not show an error alert', async ({ page }) => {
    const errorAlert = page.locator('[data-testid="memory-error"]');
    await expect(errorAlert).not.toBeVisible({ timeout: 5000 });
  });

  test('should hide the toolbar for anonymous users', async ({ page }) => {
    await expect(page.locator('[data-testid="memories-toolbar"]')).toHaveCount(0);
  });

  test('should not render the graph or the empty state', async ({ page }) => {
    // Anonymous notice replaces both
    await expect(page.locator('[data-testid="memory-graph"]')).toHaveCount(0);
    await expect(page.locator('[data-testid="empty-state"]')).toHaveCount(0);
  });

  test('should still show the consent banner', async ({ page }) => {
    const banner = page.locator('[data-testid="consent-banner"]');
    await expect(banner).toBeVisible({ timeout: 5000 });
  });

  test('should not fire a request to the memory proxy', async ({ page }) => {
    // The hook is disabled for anonymous users, so no /api/workspaces/*/memory
    // request should fire during page load.
    let memoryRequestFired = false;
    page.on('request', (req) => {
      const url = req.url();
      if (url.includes('/api/workspaces/') && url.includes('/memory')) {
        memoryRequestFired = true;
      }
    });

    await page.reload();
    await page.waitForSelector(ANONYMOUS_NOTICE, { timeout: 10000 });
    // Give any in-flight requests a chance to fire
    await page.waitForTimeout(1000);

    expect(memoryRequestFired).toBe(false);
  });
});
