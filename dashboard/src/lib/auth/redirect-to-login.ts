/**
 * Redirect the browser to the login page.
 *
 * An in-flight guard ensures only one redirect fires per page lifetime —
 * multiple concurrent callers (e.g. SessionWatcher + a 401-reacting
 * QueryCache handler) will not stack up redundant navigations.
 *
 * Safe to call from SSR contexts: returns immediately when `window` is
 * undefined.
 */

let redirecting = false;

/**
 * Navigate to /login, preserving the current URL as `returnTo`.
 *
 * @param returnTo  Override the return-to path. Defaults to
 *                  `window.location.pathname + window.location.search`.
 */
export function redirectToLogin(returnTo?: string): void {
  if (typeof window === "undefined") return;
  if (redirecting) return;
  redirecting = true;
  const path = returnTo ?? window.location.pathname + window.location.search;
  const url = `/login?returnTo=${encodeURIComponent(path)}`;
  window.location.assign(url);
}

/**
 * Reset the in-flight redirect guard.
 *
 * Exposed for testing only — do NOT call this in production code.
 */
export function _resetRedirectGuard(): void {
  redirecting = false;
}
