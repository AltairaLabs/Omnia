import { test, expect } from '../fixtures/coverage';

/**
 * E2E tests for the Deploy Agent Wizard.
 * Tests wizard navigation, form validation, and agent creation flow.
 */

// Common selectors
const SELECTORS = {
  agentCard: '[data-testid="agent-card"], [data-testid="agent-row"]',
  dialog: '[role="dialog"]',
  combobox: 'button[role="combobox"]',
  option: '[role="option"]',
  nameInput: 'input#name',
  customImageInput: 'input#customImage',
};

// Button selectors
const BUTTONS = {
  newAgent: 'button:has-text("New Agent")',
  next: 'button:has-text("Next")',
  back: 'button:has-text("Back")',
  deploy: 'button:has-text("Deploy Agent")',
};

// Step indicators
const STEPS = {
  step1: 'text=Step 1 of 7',
  step2: 'text=Step 2 of 7',
  step3: 'text=Step 3 of 7',
  step4: 'text=Step 4 of 7',
  step5: 'text=Step 5 of 7',
  step6: 'text=Step 6 of 7',
  step7: 'text=Step 7 of 7',
};

// Labels and text
const LABELS = {
  deployNewAgent: 'text=Deploy New Agent',
  agentName: 'label:has-text("Agent Name")',
  agentFramework: 'text=Agent Framework',
  promptKit: 'text=PromptKit',
  langChain: 'text=LangChain',
  autoGen: 'text=AutoGen',
  custom: 'label:has-text("Custom")',
  customText: 'text=Custom',
  promptPack: 'text=PromptPack',
  llmProvider: 'text=LLM Provider',
  selectProvider: 'text=Select a configured Provider',
  noProvidersAvailable: 'text=No Providers available',
  noProvidersConfigured: 'text=No Providers configured',
  toolRegistry: 'text=Tool Registry',
  sessionStorage: 'text=Session Storage',
  facadeType: 'text=Facade Type',
  replicas: 'text=Replicas',
  reviewConfig: 'text=Review Configuration',
  yamlPreview: 'text=YAML Preview',
  containerImage: 'text=Container Image',
};

// Test agent names
const TEST_NAMES = {
  simple: 'test-agent',
  withDashes: 'my-test-agent',
  numbered: 'test-agent-123',
  final: 'final-test-agent',
};

// Type aliases for Playwright
type PlaywrightPage = import('@playwright/test').Page;
type PlaywrightLocator = import('@playwright/test').Locator;

// Helper to open wizard
async function openWizard(page: PlaywrightPage) {
  await page.locator(BUTTONS.newAgent).click();
  return page.locator(SELECTORS.dialog);
}

// Helper to fill name and go to next step
async function fillNameAndNext(dialog: PlaywrightLocator, name: string) {
  await dialog.locator(SELECTORS.nameInput).fill(name);
  await dialog.locator(BUTTONS.next).click();
}

// Helper to select first option from combobox
async function selectFirstOption(page: PlaywrightPage, dialog: PlaywrightLocator) {
  const combobox = dialog.locator(SELECTORS.combobox).first();
  await combobox.click();
  const firstOption = page.locator(SELECTORS.option).first();
  if (await firstOption.isVisible()) {
    await firstOption.click();
  }
}

test.describe('Deploy Agent Wizard', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/agents');
    await page.waitForSelector(SELECTORS.agentCard, { timeout: 10000 });
  });

  test('should open wizard when clicking New Agent button', async ({ page }) => {
    const newAgentBtn = page.locator(BUTTONS.newAgent);
    await expect(newAgentBtn).toBeVisible();
    await newAgentBtn.click();

    const dialog = page.locator(SELECTORS.dialog);
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(LABELS.deployNewAgent)).toBeVisible();
  });

  test('should show step 1 (Basic Info) initially', async ({ page }) => {
    const dialog = await openWizard(page);

    await expect(dialog.locator(STEPS.step1)).toBeVisible();
    await expect(dialog.locator(LABELS.agentName)).toBeVisible();
    await expect(dialog.locator(SELECTORS.nameInput)).toBeVisible();
  });

  test('should validate agent name format', async ({ page }) => {
    const dialog = await openWizard(page);
    const nameInput = dialog.locator(SELECTORS.nameInput);
    const nextBtn = dialog.locator(BUTTONS.next);

    // Next should be disabled with empty name
    await expect(nextBtn).toBeDisabled();

    // Type valid name
    await nameInput.fill(TEST_NAMES.withDashes);
    await expect(nextBtn).toBeEnabled();

    // Clear and type invalid name - uppercase letters should be converted to lowercase
    await nameInput.fill('');
    await nameInput.fill('MyAgent');
    // The input should auto-convert to lowercase with hyphens
    await expect(nameInput).toHaveValue('my-agent');
  });

  test('should navigate through all wizard steps', async ({ page }) => {
    const dialog = await openWizard(page);
    const nextBtn = dialog.locator(BUTTONS.next);
    const backBtn = dialog.locator(BUTTONS.back);

    // Step 1: Basic Info
    await expect(dialog.locator(STEPS.step1)).toBeVisible();
    await fillNameAndNext(dialog, TEST_NAMES.simple);

    // Step 2: Framework
    await expect(dialog.locator(STEPS.step2)).toBeVisible();
    await expect(dialog.locator(LABELS.agentFramework)).toBeVisible();
    await expect(dialog.locator(LABELS.promptKit)).toBeVisible();
    await nextBtn.click();

    // Step 3: PromptPack
    await expect(dialog.locator(STEPS.step3)).toBeVisible();
    await expect(dialog.locator(LABELS.promptPack)).toBeVisible();
    await selectFirstOption(page, dialog);
    await nextBtn.click();

    // Step 4: Provider
    await expect(dialog.locator(STEPS.step4)).toBeVisible();
    await expect(dialog.locator(LABELS.llmProvider)).toBeVisible();
    await nextBtn.click();

    // Step 5: Options (Tools & Session)
    await expect(dialog.locator(STEPS.step5)).toBeVisible();
    await expect(dialog.locator(LABELS.toolRegistry)).toBeVisible();
    await expect(dialog.locator(LABELS.sessionStorage)).toBeVisible();
    await nextBtn.click();

    // Step 6: Runtime
    await expect(dialog.locator(STEPS.step6)).toBeVisible();
    await expect(dialog.locator(LABELS.facadeType)).toBeVisible();
    await expect(dialog.locator(LABELS.replicas)).toBeVisible();
    await nextBtn.click();

    // Step 7: Review
    await expect(dialog.locator(STEPS.step7)).toBeVisible();
    await expect(dialog.locator(LABELS.reviewConfig)).toBeVisible();
    await expect(dialog.locator(LABELS.yamlPreview)).toBeVisible();

    // Verify back button works
    await backBtn.click();
    await expect(dialog.locator(STEPS.step6)).toBeVisible();
  });

  test('should show framework options', async ({ page }) => {
    const dialog = await openWizard(page);
    await fillNameAndNext(dialog, TEST_NAMES.simple);

    // Verify all framework options are present
    await expect(dialog.locator(LABELS.promptKit)).toBeVisible();
    await expect(dialog.locator(LABELS.langChain)).toBeVisible();
    await expect(dialog.locator(LABELS.autoGen)).toBeVisible();
    await expect(dialog.locator(LABELS.customText)).toBeVisible();
  });

  test('should show custom image input when Custom framework selected', async ({ page }) => {
    const dialog = await openWizard(page);
    await fillNameAndNext(dialog, TEST_NAMES.simple);

    // Select Custom framework
    await dialog.locator(LABELS.custom).click();

    // Custom image input should appear
    await expect(dialog.locator(SELECTORS.customImageInput)).toBeVisible();
    await expect(dialog.locator(LABELS.containerImage)).toBeVisible();

    // Next should be disabled without custom image
    const nextBtn = dialog.locator(BUTTONS.next);
    await expect(nextBtn).toBeDisabled();

    // Fill custom image
    await dialog.locator(SELECTORS.customImageInput).fill('myregistry/my-agent:v1.0');
    await expect(nextBtn).toBeEnabled();
  });

  test('should show provider selection from workspace', async ({ page }) => {
    const dialog = await openWizard(page);
    const nextBtn = dialog.locator(BUTTONS.next);

    // Step 1: Name
    await fillNameAndNext(dialog, TEST_NAMES.simple);

    // Step 2: Framework (use default)
    await nextBtn.click();

    // Step 3: PromptPack - select one
    await selectFirstOption(page, dialog);
    await nextBtn.click();

    // Step 4: Provider
    await expect(dialog.locator(LABELS.llmProvider)).toBeVisible();
    await expect(dialog.locator(LABELS.selectProvider)).toBeVisible();

    // In demo mode, providers are empty - should show warning
    const noProvidersMsg = dialog.locator(LABELS.noProvidersAvailable);
    const noProvidersWarning = dialog.locator(LABELS.noProvidersConfigured);

    // Either message indicates provider list is empty
    const hasNoProviders = await noProvidersMsg.isVisible().catch(() => false) ||
                           await noProvidersWarning.isVisible().catch(() => false);

    // This is expected in demo mode - can still proceed
    if (hasNoProviders) {
      await expect(nextBtn).toBeEnabled();
    }
  });

  test('should display YAML preview on review step', async ({ page }) => {
    const dialog = await openWizard(page);
    const nextBtn = dialog.locator(BUTTONS.next);

    // Fill wizard quickly
    await dialog.locator(SELECTORS.nameInput).fill(TEST_NAMES.withDashes);
    await nextBtn.click(); // to Framework
    await nextBtn.click(); // to PromptPack

    // Select first promptpack if available
    await selectFirstOption(page, dialog);
    await nextBtn.click(); // to Provider
    await nextBtn.click(); // to Options
    await nextBtn.click(); // to Runtime
    await nextBtn.click(); // to Review

    // Verify YAML preview is shown
    await expect(dialog.locator(LABELS.yamlPreview)).toBeVisible();
    await expect(dialog.locator(LABELS.reviewConfig)).toBeVisible();

    // YAML should contain the agent name
    const yamlBlock = dialog.locator('pre, code');
    await expect(yamlBlock.first()).toContainText(TEST_NAMES.withDashes);
    await expect(yamlBlock.first()).toContainText('AgentRuntime');
  });

  test('should close wizard when pressing escape', async ({ page }) => {
    const dialog = await openWizard(page);
    await expect(dialog).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
  });

  test('should reset form when wizard is reopened', async ({ page }) => {
    const dialog = await openWizard(page);
    const nameInput = dialog.locator(SELECTORS.nameInput);

    await nameInput.fill(TEST_NAMES.numbered);
    await expect(nameInput).toHaveValue(TEST_NAMES.numbered);

    // Close wizard
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();

    // Reopen wizard
    await page.locator(BUTTONS.newAgent).click();
    await expect(dialog).toBeVisible();

    // Name should be reset to empty
    await expect(nameInput).toHaveValue('');
  });

  test('should show Deploy Agent button on final step', async ({ page }) => {
    const dialog = await openWizard(page);
    const nextBtn = dialog.locator(BUTTONS.next);

    // Navigate through all steps
    await dialog.locator(SELECTORS.nameInput).fill(TEST_NAMES.final);
    await nextBtn.click(); // Framework
    await nextBtn.click(); // PromptPack

    await selectFirstOption(page, dialog);
    await nextBtn.click(); // Provider
    await nextBtn.click(); // Options
    await nextBtn.click(); // Runtime
    await nextBtn.click(); // Review

    // On review step, should see Deploy Agent button instead of Next
    await expect(dialog.locator(BUTTONS.deploy)).toBeVisible();
    await expect(dialog.locator(BUTTONS.next)).not.toBeVisible();
  });
});
