import { test as base, expect, Page, TestInfo } from '@playwright/test';
import { addCoverageReport } from 'monocart-reporter';

/**
 * Extended test fixture for multi-modal console E2E tests.
 * Provides helpers for interacting with the agent console.
 *
 * Includes coverage collection when COLLECT_COVERAGE=true.
 */

const collectCoverage = process.env.COLLECT_COVERAGE === 'true';

// Selectors for console elements
export const SELECTORS = {
  consoleInput: '[data-testid="console-input"]',
  sendButton: '[data-testid="send-button"]',
  messageList: '[data-testid="message-list"]',
  userMessage: '[data-testid="message-user"]',
  assistantMessage: '[data-testid="message-assistant"]',
  streamingIndicator: '[data-streaming="true"]',
  connectionStatus: '[data-testid="connection-status"]',
  // Media elements
  imageAttachment: '[data-testid="image-attachment"]',
  audioPlayer: '[data-testid="audio-player"]',
  videoPlayer: '[data-testid="video-player"]',
  documentPreview: '[data-testid="document-preview"]',
  // Lightbox
  lightbox: '[data-testid="image-lightbox"]',
  lightboxClose: '[data-testid="lightbox-close"]',
  // Upload
  attachmentButton: '[data-testid="attachment-button"]',
  attachmentPreview: '[data-testid="attachment-preview"]',
  dropzone: '[data-testid="console-dropzone"]',
} as const;

// Error message constant
const NO_MESSAGE_ERROR = 'No assistant message found';

/**
 * Custom test fixture with connected console page.
 * Includes automatic coverage collection when COLLECT_COVERAGE=true.
 */
export const test = base.extend<{
  connectedConsolePage: Page;
  autoCollectCoverage: void;
}>({
  // Auto-use fixture for coverage collection
  autoCollectCoverage: [async ({ page }: { page: Page }, use: () => Promise<void>, testInfo: TestInfo) => {
    const isChromium = testInfo.project.name === 'chromium';

    if (collectCoverage && isChromium) {
      await page.coverage.startJSCoverage({ resetOnNavigation: false });
    }

    await use();

    if (collectCoverage && isChromium) {
      const coverageData = await page.coverage.stopJSCoverage();
      if (coverageData.length > 0) {
        await addCoverageReport(coverageData, testInfo);
      }
    }
  }, { auto: true }],

  connectedConsolePage: async ({ page }, use) => {
    // Navigate to the console page for the test agent
    // Route format: /agents/[name]?namespace=...&tab=console
    await page.goto('/agents/e2e-test-agent?namespace=default&tab=console');

    // Wait for WebSocket connection to establish
    await page.waitForSelector(SELECTORS.connectionStatus, { timeout: 10000 });

    // Wait for connection to be established (should show "Connected")
    await expect(page.locator(SELECTORS.connectionStatus)).toContainText('Connected', {
      timeout: 15000,
    });

    // eslint-disable-next-line react-hooks/rules-of-hooks
    await use(page);
  },
});

export { expect };

/**
 * Send a message and wait for the assistant response to complete.
 */
export async function sendMessageAndWait(page: Page, message: string): Promise<void> {
  // Type the message
  await page.fill(SELECTORS.consoleInput, message);

  // Click send button
  await page.click(SELECTORS.sendButton);

  // Wait for user message to appear
  await page.waitForSelector(`${SELECTORS.userMessage}:has-text("${message.slice(0, 20)}")`, {
    timeout: 5000,
  });

  // Wait for assistant message to appear and complete (no streaming indicator)
  await page.waitForSelector(SELECTORS.assistantMessage, { timeout: 10000 });

  // Wait for streaming to complete
  await page.waitForFunction(
    (selector) => {
      const messages = document.querySelectorAll(selector);
      const lastMessage = messages[messages.length - 1];
      return lastMessage && !lastMessage.hasAttribute('data-streaming');
    },
    SELECTORS.assistantMessage,
    { timeout: 30000 }
  );

  // Small delay to ensure all content is rendered
  await page.waitForTimeout(500);
}

/**
 * Get the last assistant message element.
 */
export async function getLastAssistantMessage(page: Page) {
  const messages = await page.locator(SELECTORS.assistantMessage).all();
  return messages[messages.length - 1];
}

/**
 * Check if an element exists in the last assistant message.
 */
export async function lastMessageContains(page: Page, selector: string): Promise<boolean> {
  const lastMessage = await getLastAssistantMessage(page);
  if (!lastMessage) return false;
  return (await lastMessage.locator(selector).count()) > 0;
}

/**
 * Simulate drag and drop file upload.
 */
export async function dragDropFile(
  page: Page,
  filePath: string,
  mimeType: string
): Promise<void> {
  const dropzone = page.locator(SELECTORS.dropzone);

  // Create a data transfer with the file
  const dataTransfer = await page.evaluateHandle(
    async ({ path, type }) => {
      const dt = new DataTransfer();
      const response = await fetch(path);
      const blob = await response.blob();
      const file = new File([blob], path.split('/').pop() || 'file', { type });
      dt.items.add(file);
      return dt;
    },
    { path: filePath, type: mimeType }
  );

  // Dispatch drop event
  await dropzone.dispatchEvent('drop', { dataTransfer });
}

/**
 * Simulate clipboard paste with an image.
 */
export async function pasteImage(page: Page, base64Data: string): Promise<void> {
  await page.evaluate(async (data) => {
    // Convert base64 to blob
    const byteCharacters = atob(data);
    const byteNumbers = new Array(byteCharacters.length);
    for (let i = 0; i < byteCharacters.length; i++) {
      byteNumbers[i] = byteCharacters.charCodeAt(i);
    }
    const byteArray = new Uint8Array(byteNumbers);
    const blob = new Blob([byteArray], { type: 'image/png' });
    const file = new File([blob], 'pasted-image.png', { type: 'image/png' });

    // Create a DataTransfer with the file
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(file);

    // Dispatch paste event with the image file
    const input = document.querySelector('[data-testid="console-input"]');
    if (input) {
      const pasteEvent = new ClipboardEvent('paste', {
        clipboardData: dataTransfer,
        bubbles: true,
      });
      input.dispatchEvent(pasteEvent);
    }
  }, base64Data);
}

/**
 * Wait for an image to load in the last message.
 */
export async function waitForImageInMessage(page: Page): Promise<void> {
  const lastMessage = await getLastAssistantMessage(page);
  if (!lastMessage) {
    throw new Error(NO_MESSAGE_ERROR);
  }

  await lastMessage.locator(SELECTORS.imageAttachment).waitFor({ timeout: 10000 });
}

/**
 * Wait for an audio player to appear in the last message.
 */
export async function waitForAudioInMessage(page: Page): Promise<void> {
  const lastMessage = await getLastAssistantMessage(page);
  if (!lastMessage) {
    throw new Error(NO_MESSAGE_ERROR);
  }

  await lastMessage.locator(SELECTORS.audioPlayer).waitFor({ timeout: 10000 });
}

/**
 * Wait for a document preview to appear in the last message.
 */
export async function waitForDocumentInMessage(page: Page): Promise<void> {
  const lastMessage = await getLastAssistantMessage(page);
  if (!lastMessage) {
    throw new Error(NO_MESSAGE_ERROR);
  }

  await lastMessage.locator(SELECTORS.documentPreview).waitFor({ timeout: 10000 });
}
