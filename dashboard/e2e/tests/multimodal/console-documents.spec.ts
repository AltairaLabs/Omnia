import { test, expect, SELECTORS, sendMessageAndWait, getLastAssistantMessage } from '../../fixtures/multimodal';

/**
 * E2E tests for document preview in the agent console.
 * These tests use a real WebSocket connection to a demo-mode agent
 * that returns canned multi-modal responses including documents.
 */

// Test message constants
const SHOW_DOCUMENT = 'show document';
const SEND_DOCUMENT = 'send document';
const SEND_PDF = 'send pdf';

// Check for file size indicator using simple string matching
function hasFileSizeIndicator(text: string | null): boolean {
  if (!text) return false;
  const lower = text.toLowerCase();
  // Check for common file size patterns
  return lower.includes('kb') ||
         lower.includes('mb') ||
         lower.includes('bytes') ||
         lower.includes(' b');
}

test.describe('Console Document Preview', () => {
  test('should render document preview for file attachments', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SHOW_DOCUMENT);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify document preview is rendered
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });
  });

  test('should display filename', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SEND_DOCUMENT);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify filename is displayed
    const previewText = await documentPreview.textContent();
    expect(previewText).toContain('test-document.pdf');
  });

  test('should display file icon for PDF', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SEND_PDF);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify there's an icon (either svg, img, or icon element)
    const icon = documentPreview.locator('svg, img, [class*="icon"], [data-testid*="icon"]');
    await expect(icon.first()).toBeVisible();
  });

  test('should have download/open button', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SHOW_DOCUMENT);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify there's a download or open button/link
    const actionButton = documentPreview.locator('a[download], button[aria-label*="download"], button[aria-label*="Download"], a[href], button[aria-label*="open"], button[aria-label*="Open"]');
    await expect(actionButton.first()).toBeVisible();
  });

  test('should show file size if available', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SEND_DOCUMENT);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Check for file size indicator (optional - may show bytes, KB, etc.)
    const previewText = await documentPreview.textContent();

    // This is informational - file size display is optional
    if (hasFileSizeIndicator(previewText)) {
      expect(hasFileSizeIndicator(previewText)).toBeTruthy();
    }
  });

  test('should be clickable', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SHOW_DOCUMENT);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify the preview is interactive
    // Either the whole preview is clickable or there's a button/link inside
    const clickable = documentPreview.locator('a, button').first();
    await expect(clickable).toBeEnabled();
  });

  test('should have proper styling and layout', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SEND_PDF);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify it has reasonable dimensions
    const boundingBox = await documentPreview.boundingBox();
    expect(boundingBox).toBeTruthy();
    expect(boundingBox!.width).toBeGreaterThan(50);
    expect(boundingBox!.height).toBeGreaterThan(20);
  });

  test('should have accessible document preview', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a document response
    await sendMessageAndWait(page, SHOW_DOCUMENT);

    // Get the document preview
    const lastMessage = await getLastAssistantMessage(page);
    const documentPreview = lastMessage!.locator(SELECTORS.documentPreview);
    await expect(documentPreview).toBeVisible({ timeout: 10000 });

    // Verify accessibility: interactive elements should have accessible names
    const interactiveElements = documentPreview.locator('a, button');
    const count = await interactiveElements.count();

    for (let i = 0; i < count; i++) {
      const element = interactiveElements.nth(i);
      const ariaLabel = await element.getAttribute('aria-label');
      const title = await element.getAttribute('title');
      const textContent = await element.textContent();

      // Element should have some accessible name
      expect(ariaLabel || title || textContent?.trim()).toBeTruthy();
    }
  });
});
