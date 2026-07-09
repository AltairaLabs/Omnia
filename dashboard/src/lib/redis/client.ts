/**
 * Redis client singleton for server-side dashboard features.
 *
 * Used by:
 * - Arena live stats SSE endpoint (reads accumulator hashes)
 * - Future: session event bus
 *
 * Configuration via environment variables:
 * - ARENA_REDIS_URL: Full Redis URL (redis://host:port)
 * - ARENA_REDIS_ADDR: host:port (fallback when URL is not set)
 * - ARENA_REDIS_PASSWORD: Optional password
 * - ARENA_REDIS_DB: Optional database number (default: 0)
 */

import Redis from "ioredis";

const globalForRedis = globalThis as unknown as { arenaRedis?: Redis };

const CONNECT_TIMEOUT_MS = 5000;
const MAX_RETRIES_PER_REQUEST = 3;
const RETRY_DELAY_STEP_MS = 200;
const RETRY_DELAY_CAP_MS = 2000;

// This is a process-lifetime singleton, so retryStrategy MUST NEVER return
// null: returning null tells ioredis to stop reconnecting for good, which
// bricks the client until the pod restarts. If Redis is briefly unavailable
// around startup, the client would give up permanently (#1810). Retry forever
// with capped backoff instead; maxRetriesPerRequest keeps individual commands
// failing fast in the meantime.
function retryStrategy(times: number): number {
  return Math.min(times * RETRY_DELAY_STEP_MS, RETRY_DELAY_CAP_MS);
}

/**
 * Returns a shared Redis client for arena features, or null if Redis is not configured.
 * The client is created lazily on first call and reused across requests.
 * Stored on globalThis to survive Next.js hot reloads in development.
 */
export function getArenaRedisClient(): Redis | null {
  if (!process.env.ARENA_REDIS_URL && !process.env.ARENA_REDIS_ADDR) {
    return null;
  }

  if (!globalForRedis.arenaRedis) {
    const url = process.env.ARENA_REDIS_URL;
    if (url) {
      globalForRedis.arenaRedis = new Redis(url, {
        connectTimeout: CONNECT_TIMEOUT_MS,
        maxRetriesPerRequest: MAX_RETRIES_PER_REQUEST,
        retryStrategy,
      });
    } else {
      const addr = process.env.ARENA_REDIS_ADDR || "localhost:6379";
      const [host, port] = addr.split(":");
      globalForRedis.arenaRedis = new Redis({
        host,
        port: Number.parseInt(port || "6379", 10),
        password: process.env.ARENA_REDIS_PASSWORD || undefined,
        db: Number.parseInt(process.env.ARENA_REDIS_DB || "0", 10),
        connectTimeout: CONNECT_TIMEOUT_MS,
        maxRetriesPerRequest: MAX_RETRIES_PER_REQUEST,
        retryStrategy,
      });
    }

    globalForRedis.arenaRedis.on("error", (err) => {
      console.error("Arena Redis client error", err);
    });
  }

  return globalForRedis.arenaRedis;
}
