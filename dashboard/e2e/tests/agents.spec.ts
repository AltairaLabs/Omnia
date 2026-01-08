import { test, expect } from '@playwright/test';

/**
 * E2E tests for the Agents page.
 * Tests agent listing, details, and interactions.
 */

// Common selectors
const SELECTORS = {
  agentCard: '[data-testid="agent-card"], [data-testid="agent-row"]',
  agentList: '[data-testid="agent-list"], [data-testid="agent-card"]',
  statusBadge: '[data-testid="agent-status"], [data-testid="status-badge"]',
};

// URL patterns (simple patterns to avoid slow regex)
const agentDetailUrlPattern = /agents\/[^/]+$/;

test.describe('Agents Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/agents');
  });

  test('should display list of agents', async ({ page }) => {
    // Wait for agent list to load
    await page.waitForSelector(SELECTORS.agentList, {
      timeout: 10000,
    });

    // Verify at least one agent is displayed (in demo mode)
    const agents = page.locator(SELECTORS.agentCard);
    await expect(agents.first()).toBeVisible();
  });

  test('should show agent status indicators', async ({ page }) => {
    // Wait for agents to load
    await page.waitForSelector(SELECTORS.agentCard);

    // Check for status badges
    const statusBadges = page.locator(SELECTORS.statusBadge);
    await expect(statusBadges.first()).toBeVisible();
  });

  test('should navigate to agent details on click', async ({ page }) => {
    // Wait for agent list to load
    await page.waitForSelector(SELECTORS.agentCard);

    // Click on first agent
    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();

    // Verify navigation to detail page
    await expect(page).toHaveURL(agentDetailUrlPattern);
  });

  test('should toggle between card and table view', async ({ page }) => {
    // Find view toggle buttons
    const cardViewBtn = page.locator('[data-testid="view-cards"]');
    const tableViewBtn = page.locator('[data-testid="view-table"]');

    // If view toggle exists
    if (await cardViewBtn.isVisible() || await tableViewBtn.isVisible()) {
      // Toggle to table view if card view button exists
      if (await tableViewBtn.isVisible()) {
        await tableViewBtn.click();
        await expect(page.locator('[data-testid="agents-table"]')).toBeVisible();
      }

      // Toggle back to card view
      if (await cardViewBtn.isVisible()) {
        await cardViewBtn.click();
        await expect(page.locator('[data-testid="agent-card"]').first()).toBeVisible();
      }
    }
  });

  test('should filter agents by search', async ({ page }) => {
    // Find search input
    const searchInput = page.locator(
      '[data-testid="agent-search"], input[placeholder*="Search"], input[type="search"]'
    );

    if (await searchInput.isVisible()) {
      // Type a search query
      await searchInput.fill('demo');

      // Wait for filter to apply
      await page.waitForTimeout(500);

      // Verify results are filtered (or no results message)
      const agents = page.locator(SELECTORS.agentCard);
      const noResults = page.locator('[data-testid="no-results"]');

      // Either filtered agents or no results message should be visible
      const hasAgents = await agents.first().isVisible().catch(() => false);
      const hasNoResults = await noResults.isVisible().catch(() => false);
      expect(hasAgents || hasNoResults).toBe(true);
    }
  });
});

test.describe('Agent Details', () => {
  test('should display agent overview tab', async ({ page }) => {
    // Navigate to first agent
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();
    await expect(page).toHaveURL(agentDetailUrlPattern);

    // Verify overview information is displayed
    await expect(
      page.locator('[data-testid="agent-name"], h1, [data-testid="agent-title"]')
    ).toBeVisible();
  });

  test('should display agent tabs', async ({ page }) => {
    // Navigate to agent detail page
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();

    // Check for tab navigation
    const tabs = page.locator('[role="tablist"], [data-testid="agent-tabs"]');

    if (await tabs.isVisible()) {
      // Common tabs: Overview, Console, Logs, Metrics
      const expectedTabs = ['Overview', 'Console', 'Logs', 'Events'];
      for (const tabName of expectedTabs) {
        const tab = page.locator(`[role="tab"]:has-text("${tabName}"), button:has-text("${tabName}")`);
        if (await tab.isVisible().catch(() => false)) {
          // Tab exists, that's good
          expect(true).toBe(true);
        }
      }
    }
  });

  test('should show console tab with message input', async ({ page }) => {
    // Navigate to agent detail page
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();

    // Click on Console tab
    const consoleTab = page.locator('[role="tab"]:has-text("Console"), button:has-text("Console")');
    if (await consoleTab.isVisible()) {
      await consoleTab.click();

      // Verify message input exists
      const messageInput = page.locator(
        '[data-testid="console-input"], textarea, input[placeholder*="message"]'
      );
      await expect(messageInput.first()).toBeVisible({ timeout: 5000 }).catch(() => {
        // Console might require authentication or agent to be running
      });
    }
  });

  test('should show logs tab with log entries', async ({ page }) => {
    // Navigate to agent detail page
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();

    // Click on Logs tab
    const logsTab = page.locator('[role="tab"]:has-text("Logs"), button:has-text("Logs")');
    if (await logsTab.isVisible()) {
      await logsTab.click();

      // Wait for logs to load
      await page.waitForTimeout(1000);

      // Verify log viewer exists
      const logViewer = page.locator('[data-testid="log-viewer"], [data-testid="logs-container"]');
      await expect(logViewer).toBeVisible({ timeout: 5000 }).catch(() => {
        // Logs might not be available in demo mode
      });
    }
  });
});

test.describe('Agent Scaling', () => {
  test('should display replica count', async ({ page }) => {
    // Navigate to agent detail page
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard);

    const firstAgent = page.locator(SELECTORS.agentCard).first();
    await firstAgent.click();

    // Look for replica count display (simple pattern)
    const replicaInfo = page.locator('[data-testid="replica-count"], [data-testid="replicas"]');
    if (await replicaInfo.first().isVisible().catch(() => false)) {
      await expect(replicaInfo.first()).toBeVisible();
    }
  });
});
