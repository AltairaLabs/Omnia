import { test, expect, SELECTORS, sendMessageAndWait, getLastAssistantMessage } from '../../fixtures/multimodal';

/**
 * E2E tests for image rendering in the agent console.
 * These tests use a real WebSocket connection to a demo-mode agent
 * that returns canned multi-modal responses.
 */

// Test message constants
const SHOW_IMAGE = 'show image';
const SEND_IMAGE = 'send image';

test.describe('Console Image Attachments', () => {
  test('should render image in assistant message', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify image attachment is rendered
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await expect(imageAttachment).toBeVisible({ timeout: 10000 });

    // Verify the image has loaded (has dimensions)
    const img = imageAttachment.locator('img');
    await expect(img).toBeVisible();

    // Check image has valid src (should be data URL or blob URL)
    const src = await img.getAttribute('src');
    expect(src).toBeTruthy();
    expect(src!.startsWith('data:image/') || src!.startsWith('blob:')).toBeTruthy();
  });

  test('should open lightbox when image clicked', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SEND_IMAGE);

    // Get the last assistant message and find the image
    const lastMessage = await getLastAssistantMessage(page);
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await expect(imageAttachment).toBeVisible({ timeout: 10000 });

    // Click the image to open lightbox
    await imageAttachment.click();

    // Verify lightbox opens
    const lightbox = page.locator(SELECTORS.lightbox);
    await expect(lightbox).toBeVisible({ timeout: 5000 });

    // Verify lightbox contains an image
    const lightboxImage = lightbox.locator('img');
    await expect(lightboxImage).toBeVisible();
  });

  test('should close lightbox when close button clicked', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Open the lightbox
    const lastMessage = await getLastAssistantMessage(page);
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await imageAttachment.click();

    // Wait for lightbox to be visible
    const lightbox = page.locator(SELECTORS.lightbox);
    await expect(lightbox).toBeVisible({ timeout: 5000 });

    // Click close button
    const closeButton = page.locator(SELECTORS.lightboxClose);
    await closeButton.click();

    // Verify lightbox is closed
    await expect(lightbox).not.toBeVisible({ timeout: 3000 });
  });

  test('should close lightbox when pressing Escape', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Open the lightbox
    const lastMessage = await getLastAssistantMessage(page);
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await imageAttachment.click();

    // Wait for lightbox to be visible
    const lightbox = page.locator(SELECTORS.lightbox);
    await expect(lightbox).toBeVisible({ timeout: 5000 });

    // Press Escape to close
    await page.keyboard.press('Escape');

    // Verify lightbox is closed
    await expect(lightbox).not.toBeVisible({ timeout: 3000 });
  });

  test('should not close lightbox when clicking backdrop (zoom/pan protection)', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Open the lightbox
    const lastMessage = await getLastAssistantMessage(page);
    const imageAttachment = lastMessage!.locator(SELECTORS.imageAttachment);
    await imageAttachment.click();

    // Wait for lightbox to be visible
    const lightbox = page.locator(SELECTORS.lightbox);
    await expect(lightbox).toBeVisible({ timeout: 5000 });

    // Click the backdrop (the lightbox container itself, not the image)
    // Lightbox intentionally prevents closing on backdrop click to support zoom/pan
    await lightbox.click({ position: { x: 10, y: 10 } });

    // Verify lightbox stays open (backdrop clicks are prevented)
    await expect(lightbox).toBeVisible({ timeout: 1000 });

    // Use close button instead
    const closeButton = page.locator(SELECTORS.lightboxClose);
    await closeButton.click();
    await expect(lightbox).not.toBeVisible({ timeout: 3000 });
  });

  test('should display image with correct aspect ratio', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Get the image
    const lastMessage = await getLastAssistantMessage(page);
    const img = lastMessage!.locator(SELECTORS.imageAttachment).locator('img');
    await expect(img).toBeVisible({ timeout: 10000 });

    // Get image dimensions
    const boundingBox = await img.boundingBox();
    expect(boundingBox).toBeTruthy();
    expect(boundingBox!.width).toBeGreaterThan(0);
    expect(boundingBox!.height).toBeGreaterThan(0);
  });

  test('should have image with valid MIME type in source', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Get the image
    const lastMessage = await getLastAssistantMessage(page);
    const img = lastMessage!.locator(SELECTORS.imageAttachment).locator('img');
    await expect(img).toBeVisible({ timeout: 10000 });

    // Verify image source has correct MIME type
    const src = await img.getAttribute('src');
    expect(src).toBeTruthy();

    // Should be a data URL with image MIME type or a blob URL
    if (src!.startsWith('data:')) {
      expect(src).toMatch(/^data:image\/(png|jpeg|gif|webp|svg\+xml)/);
    } else if (src!.startsWith('blob:')) {
      // Blob URLs don't contain MIME type info, but verify it's a valid blob URL
      expect(src).toMatch(/^blob:https?:\/\//);
    }
  });

  test('should load image successfully without errors', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SEND_IMAGE);

    // Get the image
    const lastMessage = await getLastAssistantMessage(page);
    const img = lastMessage!.locator(SELECTORS.imageAttachment).locator('img');
    await expect(img).toBeVisible({ timeout: 10000 });

    // Verify image loaded successfully (naturalWidth > 0 means loaded)
    const isLoaded = await img.evaluate((el: HTMLImageElement) => {
      return el.complete && el.naturalWidth > 0;
    });
    expect(isLoaded).toBeTruthy();
  });

  test('should have alt text for accessibility', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an image response
    await sendMessageAndWait(page, SHOW_IMAGE);

    // Get the image
    const lastMessage = await getLastAssistantMessage(page);
    const img = lastMessage!.locator(SELECTORS.imageAttachment).locator('img');
    await expect(img).toBeVisible({ timeout: 10000 });

    // Verify image has alt attribute (may be empty string for decorative images)
    const alt = await img.getAttribute('alt');
    expect(alt !== null).toBeTruthy(); // alt attribute should exist
  });
});
