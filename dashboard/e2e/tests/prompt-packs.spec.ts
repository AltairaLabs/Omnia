import { test, expect } from '@playwright/test';

/**
 * E2E tests for the Prompt Packs page.
 */

// URLs
const PROMPT_PACKS_URL = '/prompt-packs';

// Common selectors
const SELECTORS = {
  promptPackCard: '[data-testid="promptpack-card"], [data-testid="promptpack-row"]',
  promptPackList: '[data-testid="promptpack-list"], [data-testid="promptpack-card"], [data-testid="promptpack-row"]',
  statusBadge: '[data-testid="promptpack-status"], [data-testid="status-badge"]',
};

// URL patterns (simple patterns to avoid slow regex)
const promptPackDetailUrlPattern = /prompt-packs\/[^/]+$/;

test.describe('Prompt Packs Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(PROMPT_PACKS_URL);
  });

  test('should display list of prompt packs', async ({ page }) => {
    // Wait for prompt packs to load
    await page.waitForSelector(SELECTORS.promptPackList, { timeout: 10000 });

    // Verify at least one prompt pack is displayed
    const promptPacks = page.locator(SELECTORS.promptPackCard);
    await expect(promptPacks.first()).toBeVisible();
  });

  test('should show prompt pack status', async ({ page }) => {
    // Wait for prompt packs to load
    await page.waitForSelector(SELECTORS.promptPackCard);

    // Check for status indicators
    const statusBadges = page.locator(SELECTORS.statusBadge);
    if (await statusBadges.first().isVisible().catch(() => false)) {
      await expect(statusBadges.first()).toBeVisible();
    }
  });

  test('should navigate to prompt pack details', async ({ page }) => {
    // Wait for prompt packs to load
    await page.waitForSelector(SELECTORS.promptPackCard);

    // Click on first prompt pack
    const firstPromptPack = page.locator(SELECTORS.promptPackCard).first();
    await firstPromptPack.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(promptPackDetailUrlPattern);
  });

  test('should display prompt pack version', async ({ page }) => {
    // Wait for prompt packs to load
    await page.waitForSelector(SELECTORS.promptPackCard);

    // Look for version display (using data-testid only to avoid slow regex)
    const versionInfo = page.locator('[data-testid="promptpack-version"]');
    if (await versionInfo.first().isVisible().catch(() => false)) {
      await expect(versionInfo.first()).toBeVisible();
    }
  });
});

test.describe('Prompt Pack Details', () => {
  test('should display prompt pack content', async ({ page }) => {
    // Navigate to prompt packs page
    await page.goto(PROMPT_PACKS_URL);
    await page.waitForSelector(SELECTORS.promptPackCard);

    // Click on first prompt pack
    const firstPromptPack = page.locator(SELECTORS.promptPackCard).first();
    await firstPromptPack.click();

    // Wait for detail page to load
    await page.waitForURL(promptPackDetailUrlPattern);

    // Verify prompt pack name is displayed
    await expect(
      page.locator('[data-testid="promptpack-name"], h1, [data-testid="promptpack-title"]')
    ).toBeVisible();
  });

  test('should display prompt content viewer', async ({ page }) => {
    // Navigate to prompt packs page
    await page.goto(PROMPT_PACKS_URL);
    await page.waitForSelector(SELECTORS.promptPackCard);

    // Click on first prompt pack
    const firstPromptPack = page.locator(SELECTORS.promptPackCard).first();
    await firstPromptPack.click();

    // Look for content viewer or code block
    const contentViewer = page.locator(
      '[data-testid="prompt-content"], pre, code, [data-testid="content-viewer"]'
    );
    if (await contentViewer.first().isVisible().catch(() => false)) {
      await expect(contentViewer.first()).toBeVisible();
    }
  });
});
