/**
 * Factory + re-exports for the OAuth session store.
 *
 * Backend is chosen by OMNIA_SESSION_STORE:
 *   "memory" (default) — in-process store; single-replica only.
 *   "redis"            — shared Redis; required for multi-replica deployments.
 *
 * When "redis" is selected but no Redis endpoint is configured,
 * construction fails loudly at first use rather than silently falling
 * back to memory (which would turn into an inconsistency between replicas).
 */

import { MemorySessionStore } from "./memory-store";
import { RedisSessionStore } from "./redis-store";
import { getSessionRedisClient } from "./redis-client";
import type { SessionStore } from "./types";

type Backend = "memory" | "redis";

let cached: SessionStore | null = null;

function resolveBackend(): Backend {
  const raw = (process.env.OMNIA_SESSION_STORE ?? "memory").toLowerCase();
  if (raw === "memory" || raw === "redis") return raw;
  console.warn(`Unknown OMNIA_SESSION_STORE="${raw}", falling back to memory`);
  return "memory";
}

export function getSessionStore(): SessionStore {
  if (cached) return cached;

  const backend = resolveBackend();
  if (backend === "redis") {
    const redis = getSessionRedisClient();
    if (!redis) {
      throw new Error(
        "OMNIA_SESSION_STORE=redis but neither OMNIA_SESSION_REDIS_URL nor OMNIA_SESSION_REDIS_ADDR is set",
      );
    }
    cached = new RedisSessionStore(redis);
  } else {
    cached = new MemorySessionStore();
  }
  return cached;
}

/** Reset the memoised store. Test-only. */
export function __resetSessionStoreForTests(): void {
  cached = null;
}

export type { SessionStore, SessionRecord, PkceRecord } from "./types";
export { MemorySessionStore } from "./memory-store";
export { RedisSessionStore } from "./redis-store";
