/**
 * fetch with an abort-after-timeout guard.
 *
 * The session-api / memory-api proxy routes used to do a raw
 * `await fetch(url)` with no timeout — a hung or endpoint-less backend hung
 * the request (and the browser's loading state) forever, with no error to
 * trigger the page's error-branch UI. This wraps fetch with an
 * AbortController so a dead backend fails fast with a recognizable error
 * instead of hanging indefinitely.
 */
export async function fetchWithTimeout(
  url: string,
  init: RequestInit = {},
  timeoutMs = 6000
): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);

  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } catch (error) {
    if (controller.signal.aborted) {
      throw new Error("upstream timeout");
    }
    throw error;
  } finally {
    clearTimeout(timer);
  }
}
