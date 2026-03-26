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
        connectTimeout: 5000,
        maxRetriesPerRequest: 3,
        retryStrategy(times: number) {
          if (times > 5) return null;
          return Math.min(times * 200, 2000);
        },
      });
    } else {
      const addr = process.env.ARENA_REDIS_ADDR || "localhost:6379";
      const [host, port] = addr.split(":");
      globalForRedis.arenaRedis = new Redis({
        host,
        port: Number.parseInt(port || "6379", 10),
        password: process.env.ARENA_REDIS_PASSWORD || undefined,
        db: Number.parseInt(process.env.ARENA_REDIS_DB || "0", 10),
        connectTimeout: 5000,
        maxRetriesPerRequest: 3,
        retryStrategy(times: number) {
          if (times > 5) return null;
          return Math.min(times * 200, 2000);
        },
      });
    }

    globalForRedis.arenaRedis.on("error", (err) => {
      console.error("Arena Redis client error", err);
    });
  }

  return globalForRedis.arenaRedis;
}
