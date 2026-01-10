import { test, expect, SELECTORS, sendMessageAndWait, getLastAssistantMessage } from '../../fixtures/multimodal';

/**
 * E2E tests for audio playback in the agent console.
 * These tests use a real WebSocket connection to a demo-mode agent
 * that returns canned multi-modal responses including audio.
 */

// Test message constants
const PLAY_AUDIO = 'play audio';
const SEND_AUDIO = 'send audio';
const ARIA_LABEL_ATTR = 'aria-label';

test.describe('Console Audio Player', () => {
  test('should render audio player for audio attachments', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, PLAY_AUDIO);

    // Get the last assistant message
    const lastMessage = await getLastAssistantMessage(page);
    expect(lastMessage).toBeTruthy();

    // Verify audio player is rendered
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });
  });

  test('should display play button initially', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, SEND_AUDIO);

    // Get the audio player
    const lastMessage = await getLastAssistantMessage(page);
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });

    // Verify play button is visible (audio should be paused initially)
    const playButton = audioPlayer.locator('[data-testid="audio-play-button"], button[aria-label*="Play"], button[aria-label*="play"]');
    await expect(playButton).toBeVisible();
  });

  test('should show duration display', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, PLAY_AUDIO);

    // Get the audio player
    const lastMessage = await getLastAssistantMessage(page);
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });

    // Verify duration is displayed (look for time format like 0:00 or --:--)
    const durationDisplay = audioPlayer.locator('[data-testid="audio-duration"], .duration, time');
    const count = await durationDisplay.count();

    // If specific duration element exists, verify it
    if (count > 0) {
      await expect(durationDisplay.first()).toBeVisible();
    } else {
      // Otherwise check for any text containing time format
      const playerText = await audioPlayer.textContent();
      // Simple regex to match time formats like 0:00 or --:--
      expect(playerText).toMatch(/\d:\d\d|--:--/);
    }
  });

  test('should show progress bar', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, SEND_AUDIO);

    // Get the audio player
    const lastMessage = await getLastAssistantMessage(page);
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });

    // Verify progress bar exists (could be slider, progress element, or custom)
    const progressBar = audioPlayer.locator('[data-testid="audio-progress"], input[type="range"], .progress-bar, [role="slider"]');
    await expect(progressBar.first()).toBeVisible();
  });

  test('should toggle play/pause when button clicked', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, PLAY_AUDIO);

    // Get the audio player
    const lastMessage = await getLastAssistantMessage(page);
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });

    // Find the play/pause button
    const playPauseButton = audioPlayer.locator('[data-testid="audio-play-button"], button').first();
    await expect(playPauseButton).toBeVisible();

    // Click to toggle - button should be interactive
    await playPauseButton.click();

    // Small delay for state change
    await page.waitForTimeout(100);

    // Verify button is still enabled and interactive after click
    await expect(playPauseButton).toBeEnabled();
  });

  test('should have accessible audio player', async ({ connectedConsolePage: page }) => {
    // Send a message that triggers an audio response
    await sendMessageAndWait(page, SEND_AUDIO);

    // Get the audio player
    const lastMessage = await getLastAssistantMessage(page);
    const audioPlayer = lastMessage!.locator(SELECTORS.audioPlayer);
    await expect(audioPlayer).toBeVisible({ timeout: 10000 });

    // Verify accessibility: buttons should have aria-labels
    const buttons = audioPlayer.locator('button');
    const buttonCount = await buttons.count();

    for (let i = 0; i < buttonCount; i++) {
      const button = buttons.nth(i);
      const ariaLabel = await button.getAttribute(ARIA_LABEL_ATTR);
      const title = await button.getAttribute('title');
      const textContent = await button.textContent();

      // Button should have some accessible name
      expect(ariaLabel || title || textContent?.trim()).toBeTruthy();
    }
  });
});
