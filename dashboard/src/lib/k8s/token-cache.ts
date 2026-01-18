/**
 * LRU cache for workspace ServiceAccount tokens.
 *
 * Caches SA tokens to avoid repeated TokenRequest API calls.
 * Uses a simple LRU eviction strategy with TTL-based expiration.
 *
 * Tokens are cached with a 50-minute TTL (before 1hr token expiry)
 * to ensure they're refreshed before expiration.
 */

import type { WorkspaceRole } from "@/types/workspace";

/** Cache entry with expiration timestamp */
interface TokenCacheEntry {
  token: string;
  expiresAt: number;
}

/** Time-to-live for cache entries (50 minutes - before 1hr token expiry) */
const DEFAULT_TTL_MS = 50 * 60 * 1000;

/** Maximum number of entries before LRU eviction */
const MAX_ENTRIES = 100;

/** Safety margin before token expiry (5 minutes) */
const EXPIRY_MARGIN_MS = 5 * 60 * 1000;

/**
 * The token cache.
 * Uses a Map which maintains insertion order for LRU eviction.
 * Key format: "workspace:role"
 */
const cache = new Map<string, TokenCacheEntry>();

/**
 * Generate a cache key from workspace name and role.
 */
function getCacheKey(workspace: string, role: WorkspaceRole): string {
  return `${workspace}:${role}`;
}

/**
 * Check if a cache entry has expired or is about to expire.
 * We consider a token expired if it will expire within the safety margin.
 */
function isExpiredOrExpiring(entry: TokenCacheEntry): boolean {
  return Date.now() + EXPIRY_MARGIN_MS >= entry.expiresAt;
}

/**
 * Get cached token if available and not expired/expiring.
 *
 * @param workspace - Workspace name
 * @param role - Workspace role (owner, editor, viewer)
 * @returns Cached token or null if not cached/expired
 */
export function getCachedToken(
  workspace: string,
  role: WorkspaceRole
): string | null {
  const key = getCacheKey(workspace, role);
  const entry = cache.get(key);

  if (!entry) {
    return null;
  }

  if (isExpiredOrExpiring(entry)) {
    // Remove expired/expiring entry
    cache.delete(key);
    return null;
  }

  // Move to end for LRU (delete and re-add)
  cache.delete(key);
  cache.set(key, entry);

  return entry.token;
}

/**
 * Store a token in the cache.
 *
 * @param workspace - Workspace name
 * @param role - Workspace role (owner, editor, viewer)
 * @param token - The SA token to cache
 * @param expiresAt - Token expiration timestamp (ms since epoch), or undefined to use default TTL
 */
export function setCachedToken(
  workspace: string,
  role: WorkspaceRole,
  token: string,
  expiresAt?: number
): void {
  const key = getCacheKey(workspace, role);

  // If key exists, delete it first to update its position
  cache.delete(key);

  // Evict oldest entries if at capacity
  while (cache.size >= MAX_ENTRIES) {
    // Map.keys().next() gives the oldest entry (first inserted)
    const oldestKey = cache.keys().next().value;
    if (oldestKey) {
      cache.delete(oldestKey);
    }
  }

  cache.set(key, {
    token,
    expiresAt: expiresAt ?? Date.now() + DEFAULT_TTL_MS,
  });
}

/**
 * Invalidate all cached tokens for a specific workspace.
 * Call this when workspace is deleted or SAs are regenerated.
 *
 * @param workspace - Workspace name to invalidate
 */
export function invalidateWorkspaceTokens(workspace: string): void {
  const prefix = `${workspace}:`;
  const keysToDelete: string[] = [];

  for (const key of cache.keys()) {
    if (key.startsWith(prefix)) {
      keysToDelete.push(key);
    }
  }

  for (const key of keysToDelete) {
    cache.delete(key);
  }
}

/**
 * Invalidate a specific token (e.g., on auth error).
 *
 * @param workspace - Workspace name
 * @param role - Workspace role
 */
export function invalidateToken(workspace: string, role: WorkspaceRole): void {
  const key = getCacheKey(workspace, role);
  cache.delete(key);
}

/**
 * Clear all cached tokens.
 */
export function clearTokenCache(): void {
  cache.clear();
}

/**
 * Get cache statistics for monitoring.
 *
 * @returns Cache size and configuration
 */
export function getTokenCacheStats(): {
  size: number;
  maxSize: number;
  defaultTtlMs: number;
} {
  return {
    size: cache.size,
    maxSize: MAX_ENTRIES,
    defaultTtlMs: DEFAULT_TTL_MS,
  };
}

/**
 * Remove all expired entries from the cache.
 * Can be called periodically to clean up stale entries.
 *
 * @returns Number of entries removed
 */
export function pruneExpiredTokens(): number {
  const keysToDelete: string[] = [];

  for (const [key, entry] of cache.entries()) {
    if (isExpiredOrExpiring(entry)) {
      keysToDelete.push(key);
    }
  }

  for (const key of keysToDelete) {
    cache.delete(key);
  }

  return keysToDelete.length;
}
