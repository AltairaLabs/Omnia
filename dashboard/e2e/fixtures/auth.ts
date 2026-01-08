import { test as base, expect } from '@playwright/test';

/**
 * Authentication fixtures for E2E tests.
 * Provides different user contexts (anonymous, viewer, editor, admin).
 */

type AuthFixtures = {
  /** Anonymous user (no authentication) */
  anonymousPage: typeof base;
  /** Authenticated user with viewer role */
  viewerPage: typeof base;
  /** Authenticated user with editor role */
  editorPage: typeof base;
  /** Authenticated user with admin role */
  adminPage: typeof base;
};

// Extend base test with auth fixtures
export const test = base.extend<AuthFixtures>({
  // Anonymous mode - no login required
  anonymousPage: async ({ page }, callback) => {
    await callback(page as unknown as typeof base);
  },

  // Viewer role - read-only access
  viewerPage: async ({ page }, callback) => {
    // Login as viewer
    await loginAs(page, 'viewer@example.com', 'viewer123');
    await callback(page as unknown as typeof base);
  },

  // Editor role - can modify agents
  editorPage: async ({ page }, callback) => {
    // Login as editor
    await loginAs(page, 'editor@example.com', 'editor123');
    await callback(page as unknown as typeof base);
  },

  // Admin role - full access
  adminPage: async ({ page }, callback) => {
    // Login as admin
    await loginAs(page, 'admin@example.com', 'admin123');
    await callback(page as unknown as typeof base);
  },
});

/**
 * Login helper function.
 */
async function loginAs(page: import('@playwright/test').Page, email: string, password: string) {
  await page.goto('/login');

  // Wait for login form to be visible
  await page.waitForSelector('[data-testid="login-form"]', { timeout: 10000 }).catch(() => {
    // If login form not found, check if we're redirected (already logged in or anonymous mode)
    return;
  });

  // Check if we're on the login page
  if (page.url().includes('/login')) {
    await page.fill('[data-testid="email-input"]', email);
    await page.fill('[data-testid="password-input"]', password);
    await page.click('[data-testid="login-button"]');

    // Wait for redirect after login
    await page.waitForURL((url) => !url.pathname.includes('/login'), {
      timeout: 10000,
    });
  }
}

export { expect };
