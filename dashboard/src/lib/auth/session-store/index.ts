/**
 * Factory + re-exports for the OAuth session store.
 *
 * Backend is chosen by URL presence:
 *   OMNIA_SESSION_REDIS_URL set → RedisSessionStore (multi-replica safe).
 *   OMNIA_SESSION_REDIS_URL unset → MemorySessionStore (single-replica only).
 *
 * The chart's omnia.validateSession render-time guard fails install
 * when dashboard.replicaCount > 1 and no Redis URL resolves, so getting
 * here with replicaCount > 1 + memory store is impossible by construction.
 */

import { MemorySessionStore } from "./memory-store";
import { RedisSessionStore } from "./redis-store";
import { getSessionRedisClient } from "./redis-client";
import type { SessionStore } from "./types";

let cached: SessionStore | null = null;

export function getSessionStore(): SessionStore {
  if (cached) return cached;
  const redis = getSessionRedisClient();
  if (redis) {
    cached = new RedisSessionStore(redis);
    console.log("session store: redis");
  } else {
    cached = new MemorySessionStore();
    console.log("session store: memory (single-replica only)");
  }
  return cached;
}

/** Reset the memoised store. Test-only. */
export function __resetSessionStoreForTests(): void {
  cached = null;
}

export type { SessionStore, SessionRecord, PkceRecord, CliFlowRecord, CliCodeRecord } from "./types";
export { MemorySessionStore } from "./memory-store";
export { RedisSessionStore } from "./redis-store";
