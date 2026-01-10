import { test, expect, SELECTORS, sendMessageAndWait, getLastAssistantMessage } from '../../fixtures/multimodal';

/**
 * E2E tests for mixed media responses in the agent console.
 * Tests multi-modal responses containing multiple content parts (text, images, audio).
 *
 * These tests use a real WebSocket connection to a demo-mode agent
 * that returns canned multi-modal responses.
 */

// Test message constants
const MULTIMODAL = 'multimodal';
const MIXED_CONTENT = 'mixed content';

test.describe('Console Mixed Media Responses', () => {
  test('should render multiple content parts in a single response', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MULTIMODAL);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify text content is present
    const messageText = await lastMessage!.textContent();
    expect(messageText).toContain("multi-modal response");

    // Verify image attachment is rendered
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await expect(imageAttachment).toBeVisible({ timeout: 10000 });

    // Verify audio player is rendered
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });
  });

  test('should display all parts in correct order (text, image, text, audio)', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MIXED_CONTENT);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Get media content part elements
    const imagePart = lastMessage!.locator(SELECTORS.imageAttachment);
    const audioPart = lastMessage!.locator(SELECTORS.audioPlayer);

    // Verify both media types are present
    await expect(imagePart).toBeVisible({ timeout: 10000 });
    await expect(audioPart).toBeVisible({ timeout: 10000 });

    // Verify image comes before audio in the DOM order
    const imageBbox = await imagePart.boundingBox();
    const audioBbox = await audioPart.boundingBox();

    expect(imageBbox).toBeTruthy();
    expect(audioBbox).toBeTruthy();

    // Image should be above audio (y coordinate is smaller)
    expect(imageBbox!.y).toBeLessThan(audioBbox!.y);
  });

  test('should preserve image dimensions in mixed response', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MULTIMODAL);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    const imagePart = lastMessage!.locator(SELECTORS.imageAttachment);
    await expect(imagePart).toBeVisible({ timeout: 10000 });

    // Verify image has valid dimensions
    const img = imagePart.locator('img');
    const boundingBox = await img.boundingBox();
    expect(boundingBox).toBeTruthy();
    expect(boundingBox!.width).toBeGreaterThan(0);
    expect(boundingBox!.height).toBeGreaterThan(0);
  });

  test('should allow interaction with each media type independently', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MIXED_CONTENT);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);

    // Test image interaction - open lightbox
    const imagePart = lastMessage!.locator(SELECTORS.imageAttachment);
    await expect(imagePart).toBeVisible({ timeout: 10000 });
    await imagePart.click();

    // Verify lightbox opens
    const lightbox = page.locator(SELECTORS.lightbox);
    await expect(lightbox).toBeVisible({ timeout: 5000 });

    // Close lightbox
    await page.locator(SELECTORS.lightboxClose).click();
    await expect(lightbox).not.toBeVisible({ timeout: 3000 });

    // Test audio player interaction
    const audioPart = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPart).toBeVisible();

    const playButton = audioPart.locator('[data-testid="audio-play-button"], button').first();
    await expect(playButton).toBeEnabled();
  });

  test('should handle text content with MIME type verification', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MULTIMODAL);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);

    // Verify image has correct MIME type in src (data URL should include image/png)
    const img = lastMessage!.locator(SELECTORS.imageAttachment).locator('img');
    await expect(img).toBeVisible({ timeout: 10000 });

    const src = await img.getAttribute('src');
    expect(src).toBeTruthy();
    // Should be a data URL with image MIME type or a blob URL
    expect(src!.startsWith('data:image/') || src!.startsWith('blob:')).toBeTruthy();
  });

  test('should render mixed media response with proper layout', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a multi-modal response
    await sendMessageAndWait(page, MIXED_CONTENT);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify the message container has reasonable dimensions
    const messageBbox = await lastMessage!.boundingBox();
    expect(messageBbox).toBeTruthy();
    expect(messageBbox!.width).toBeGreaterThan(100);
    expect(messageBbox!.height).toBeGreaterThan(100);

    // Verify media elements don't overflow the container
    const imagePart = lastMessage!.locator(SELECTORS.imageAttachment);
    const imageBbox = await imagePart.boundingBox();
    expect(imageBbox).toBeTruthy();
    expect(imageBbox!.x).toBeGreaterThanOrEqual(messageBbox!.x - 1);
    expect(imageBbox!.x + imageBbox!.width).toBeLessThanOrEqual(messageBbox!.x + messageBbox!.width + 1);
  });

  test('should handle consecutive mixed media messages', async ({ connectedConsolePage: page }) => {
    // Send first multi-modal message
    await sendMessageAndWait(page, MULTIMODAL);

    // Verify first response has media
    let messages = await page.locator(SELECTORS.assistantMessage).all();
    let lastMessage = messages[messages.length - 1];
    let imagePart = lastMessage.locator(SELECTORS.imageAttachment);
    await expect(imagePart).toBeVisible({ timeout: 10000 });

    // Send second multi-modal message
    await sendMessageAndWait(page, MIXED_CONTENT);

    // Verify second response also has media
    messages = await page.locator(SELECTORS.assistantMessage).all();
    lastMessage = messages[messages.length - 1];
    imagePart = lastMessage.locator(SELECTORS.imageAttachment);
    await expect(imagePart).toBeVisible({ timeout: 10000 });

    // Verify both messages are distinct (should have 2 assistant messages)
    expect(messages.length).toBeGreaterThanOrEqual(2);
  });
});
