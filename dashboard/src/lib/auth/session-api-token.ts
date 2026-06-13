/**
 * Server-only reader for the dashboard pod's Kubernetes ServiceAccount token.
 *
 * The Go services (and their clients) authenticate to session-api / memory-api
 * with a ServiceAccount bearer token read from the projected token file. The
 * dashboard runs server-side (Next.js route handlers) in a pod with its own
 * ServiceAccount, so it can read the same projected token and forward it on its
 * outbound proxy calls to those backends.
 *
 * Behaviour:
 * - Reads the token from `SESSION_API_TOKEN_PATH` (default the standard
 *   projected SA path). The file is mounted only in-cluster.
 * - Caches the value with a short TTL and re-reads on expiry — the kubelet
 *   rotates the projected token periodically.
 * - When the file is missing/unreadable (local dev, dashboard E2E), returns ""
 *   and never throws. Callers then send no Authorization header, which is a
 *   no-op whether or not backend SA auth is enabled.
 *
 * This module uses Node `fs` and must only be imported from server-side code
 * (route handlers, server components, data libs). It is never bundled for the
 * browser.
 */

import { readFileSync } from "node:fs";

/** Standard projected ServiceAccount token path inside a pod. */
const DEFAULT_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token";

/** How long a read token is reused before re-reading from disk (ms). */
const TOKEN_TTL_MS = 60_000;

interface TokenCache {
  /** Last value read (trimmed). "" means "no token available". */
  value: string;
  /** Epoch ms after which the cache is considered stale. */
  expiresAt: number;
}

let cache: TokenCache | null = null;

function tokenPath(): string {
  return process.env.SESSION_API_TOKEN_PATH || DEFAULT_TOKEN_PATH;
}

function readTokenFromDisk(): string {
  try {
    return readFileSync(tokenPath(), "utf-8").trim();
  } catch {
    // File absent/unreadable (local dev) — no-op: send no Authorization header.
    return "";
  }
}

/**
 * Return the dashboard pod's ServiceAccount bearer token, or "" when none is
 * available. Cached for {@link TOKEN_TTL_MS}; re-read from disk on expiry.
 */
export function getSessionApiToken(now: number = Date.now()): string {
  if (cache && now < cache.expiresAt) {
    return cache.value;
  }
  const value = readTokenFromDisk();
  cache = { value, expiresAt: now + TOKEN_TTL_MS };
  return value;
}

/** Reset the cached token. Test-only. */
export function resetSessionApiTokenCache(): void {
  cache = null;
}

/**
 * Build outbound headers for a session-api / memory-api request, attaching the
 * ServiceAccount bearer token when one is available. When no token is present
 * (local dev), the returned headers are exactly `extra` — a safe no-op.
 *
 * @param extra Base headers (e.g. `{ Accept: "application/json" }`).
 */
export function serviceApiHeaders(
  extra?: Record<string, string>,
): Record<string, string> {
  const headers: Record<string, string> = { ...extra };
  const token = getSessionApiToken();
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}
