/**
 * LRU cache for workspace authorization decisions.
 *
 * Caches authorization results to avoid repeated K8s API calls.
 * Uses a simple LRU eviction strategy with TTL-based expiration.
 */

import type { WorkspaceAccess } from "@/types/workspace";

/** Cache entry with timestamp for TTL checking */
interface CacheEntry {
  access: WorkspaceAccess;
  timestamp: number;
}

/** Time-to-live for cache entries (5 minutes) */
const TTL_MS = 5 * 60 * 1000;

/** Maximum number of entries before LRU eviction */
const MAX_ENTRIES = 1000;

/**
 * The authorization cache.
 * Uses a Map which maintains insertion order for LRU eviction.
 * Key format: "email:workspace"
 */
const cache = new Map<string, CacheEntry>();

/**
 * Generate a cache key from email and workspace name.
 */
function getCacheKey(email: string, workspace: string): string {
  return `${email}:${workspace}`;
}

/**
 * Check if a cache entry has expired.
 */
function isExpired(entry: CacheEntry): boolean {
  return Date.now() - entry.timestamp > TTL_MS;
}

/**
 * Get cached authorization result if available and not expired.
 *
 * @param email - User email address
 * @param workspace - Workspace name
 * @returns Cached access result or null if not cached/expired
 */
export function getCachedAccess(
  email: string,
  workspace: string
): WorkspaceAccess | null {
  const key = getCacheKey(email, workspace);
  const entry = cache.get(key);

  if (!entry) {
    return null;
  }

  if (isExpired(entry)) {
    // Remove expired entry
    cache.delete(key);
    return null;
  }

  // Move to end for LRU (delete and re-add)
  cache.delete(key);
  cache.set(key, entry);

  return entry.access;
}

/**
 * Store an authorization result in the cache.
 *
 * @param email - User email address
 * @param workspace - Workspace name
 * @param access - Authorization result to cache
 */
export function setCachedAccess(
  email: string,
  workspace: string,
  access: WorkspaceAccess
): void {
  const key = getCacheKey(email, workspace);

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
    access,
    timestamp: Date.now(),
  });
}

/**
 * Invalidate all cached entries for a specific workspace.
 * Call this when workspace membership changes.
 *
 * @param workspace - Workspace name to invalidate
 */
export function invalidateWorkspaceCache(workspace: string): void {
  const suffix = `:${workspace}`;
  const keysToDelete: string[] = [];

  for (const key of cache.keys()) {
    if (key.endsWith(suffix)) {
      keysToDelete.push(key);
    }
  }

  for (const key of keysToDelete) {
    cache.delete(key);
  }
}

/**
 * Invalidate all cached entries for a specific user.
 * Call this when user group membership changes.
 *
 * @param email - User email to invalidate
 */
export function invalidateUserCache(email: string): void {
  const prefix = `${email}:`;
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
 * Clear all cached authorization decisions.
 */
export function clearAuthzCache(): void {
  cache.clear();
}

/**
 * Get cache statistics for monitoring.
 *
 * @returns Cache size and TTL configuration
 */
export function getCacheStats(): {
  size: number;
  maxSize: number;
  ttlMs: number;
} {
  return {
    size: cache.size,
    maxSize: MAX_ENTRIES,
    ttlMs: TTL_MS,
  };
}

/**
 * Remove all expired entries from the cache.
 * Can be called periodically to clean up stale entries.
 *
 * @returns Number of entries removed
 */
export function pruneExpiredEntries(): number {
  const keysToDelete: string[] = [];

  for (const [key, entry] of cache.entries()) {
    if (isExpired(entry)) {
      keysToDelete.push(key);
    }
  }

  for (const key of keysToDelete) {
    cache.delete(key);
  }

  return keysToDelete.length;
}
