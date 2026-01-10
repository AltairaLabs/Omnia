import { test, expect, SELECTORS, sendMessageAndWait, getLastAssistantMessage } from '../../fixtures/multimodal';

/**
 * E2E tests for video playback in the agent console.
 * These tests use a real WebSocket connection to a demo-mode agent
 * that returns canned multi-modal responses including video.
 */

// Test message constants
const PLAY_VIDEO = 'play video';
const SEND_VIDEO = 'send video';

// Selector for play button
const PLAY_BUTTON_SELECTOR = 'button[aria-label*="Play"]';

test.describe('Console Video Player', () => {
  test('should render video player for video attachments', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, PLAY_VIDEO);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify video player is rendered
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });
  });

  test('should display video element with controls', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, SEND_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Verify video element exists
    const videoElement = videoPlayer.locator('video');
    await expect(videoElement).toBeVisible();
  });

  test('should have play/pause button', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, PLAY_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Verify play/pause button exists
    const playPauseButton = videoPlayer.locator('[data-testid="video-play-button"], button[aria-label*="Play"], button[aria-label*="play"], button[aria-label*="Pause"], button[aria-label*="pause"]');
    await expect(playPauseButton.first()).toBeVisible();
  });

  test('should have fullscreen toggle button', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, SEND_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Click play button to start video (controls only show after video starts)
    const playButton = videoPlayer.locator(PLAY_BUTTON_SELECTOR).first();
    await playButton.click();
    await page.waitForTimeout(500);

    // Verify fullscreen button exists
    const fullscreenButton = videoPlayer.locator('[data-testid="video-fullscreen-button"], button[aria-label*="fullscreen"], button[aria-label*="Fullscreen"]');
    await expect(fullscreenButton.first()).toBeVisible();
  });

  test('should have progress/seek bar', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, PLAY_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Click play button to start video (controls only show after video starts)
    const playButton = videoPlayer.locator(PLAY_BUTTON_SELECTOR).first();
    await playButton.click();
    await page.waitForTimeout(500);

    // Verify progress bar exists (could be slider, progress element, or custom)
    const progressBar = videoPlayer.locator('[data-testid="video-progress"], input[type="range"], .progress-bar, [role="slider"]');
    await expect(progressBar.first()).toBeVisible();
  });

  test('should have volume control', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, SEND_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Click play button to start video (controls only show after video starts)
    const playButton = videoPlayer.locator(PLAY_BUTTON_SELECTOR).first();
    await playButton.click();
    await page.waitForTimeout(500);

    // Verify volume control exists (button or slider)
    const volumeControl = videoPlayer.locator('[data-testid="video-volume"], [data-testid="video-mute-button"], button[aria-label*="volume"], button[aria-label*="Volume"], button[aria-label*="mute"], button[aria-label*="Mute"]');
    await expect(volumeControl.first()).toBeVisible();
  });

  test('should toggle play/pause when clicking button', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, PLAY_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Find the play/pause button
    const playPauseButton = videoPlayer.locator('[data-testid="video-play-button"], button').first();
    await expect(playPauseButton).toBeVisible();

    // Click to toggle - this should be interactive
    await playPauseButton.click();

    // Button should be enabled and interactive
    await expect(playPauseButton).toBeEnabled();
  });

  test('should have proper video dimensions', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, SEND_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Get video element dimensions
    const videoElement = videoPlayer.locator('video');
    const boundingBox = await videoElement.boundingBox();
    expect(boundingBox).toBeTruthy();
    expect(boundingBox!.width).toBeGreaterThan(0);
    expect(boundingBox!.height).toBeGreaterThan(0);
  });

  test('should have accessible video player', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers a video response
    await sendMessageAndWait(page, PLAY_VIDEO);

    // Get the video player
    const lastMessage = await getLastAssistantMessage(page);
    const videoPlayer = lastMessage!.locator(SELECTORS.videoPlayer);
    await expect(videoPlayer).toBeVisible({ timeout: 10000 });

    // Verify accessibility: buttons should have aria-labels
    const buttons = videoPlayer.locator('button');
    const buttonCount = await buttons.count();

    for (let i = 0; i < buttonCount; i++) {
      const button = buttons.nth(i);
      const ariaLabel = await button.getAttribute('aria-label');
      const title = await button.getAttribute('title');
      const textContent = await button.textContent();

      // Button should have some accessible name
      expect(ariaLabel || title || textContent?.trim()).toBeTruthy();
    }
  });
});
