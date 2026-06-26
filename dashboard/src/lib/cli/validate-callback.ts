/** Loopback callback validation for the CLI browser-login handoff. */

const LOOPBACK_HOSTS = new Set(["127.0.0.1", "localhost", "[::1]"]);
const STATE_RE = /^[A-Za-z0-9._~-]+$/;

/**
 * Parse and validate a CLI loopback callback. Returns the parsed URL only for
 * http loopback addresses that carry an explicit port; null otherwise.
 */
export function parseLoopbackCallback(raw: string | null): URL | null {
  if (!raw) return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.protocol !== "http:") return null;
  if (!LOOPBACK_HOSTS.has(url.hostname)) return null;
  if (!url.port) return null;
  return url;
}

/** A CLI state nonce: URL-safe, 8..256 chars. */
export function isValidCliState(s: string | null): s is string {
  return !!s && s.length >= 8 && s.length <= 256 && STATE_RE.test(s);
}
