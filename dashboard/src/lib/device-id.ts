/**
 * Browser-local device identity for anonymous users.
 *
 * Client-safe — no Node.js crypto dependency.
 */

const DEVICE_ID_KEY = "omnia-device-id";

/**
 * Returns a stable per-browser device ID for anonymous users.
 * Generated once and stored in localStorage. Used as a fallback user identity
 * when no authenticated user ID is available (e.g. dev mode with anonymous access).
 */
export function getDeviceId(): string {
  if (typeof globalThis.localStorage === "undefined") {
    return "";
  }
  let id = localStorage.getItem(DEVICE_ID_KEY);
  if (!id) {
    id = crypto.randomUUID();
    localStorage.setItem(DEVICE_ID_KEY, id);
  }
  return id;
}
