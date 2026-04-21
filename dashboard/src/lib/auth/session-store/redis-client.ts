/**
 * Redis client singleton for the OAuth session store.
 *
 * Mirrors the pattern in `dashboard/src/lib/redis/client.ts` (arena
 * stats) but uses its own env-var namespace so operators can point
 * session storage at a different Redis — in particular, a managed,
 * multi-AZ Redis when the in-cluster dev Redis is not suitable.
 *
 * Env vars:
 *   OMNIA_SESSION_REDIS_URL      — full redis:// URL (preferred)
 *   OMNIA_SESSION_REDIS_ADDR     — host:port fallback
 *   OMNIA_SESSION_REDIS_PASSWORD — optional
 *   OMNIA_SESSION_REDIS_DB       — optional DB number (default 0)
 */

import Redis from "ioredis";

const globalForRedis = globalThis as unknown as { sessionRedis?: Redis };

const CONNECT_TIMEOUT_MS = 5000;
const MAX_RETRIES_PER_REQUEST = 3;
const RETRY_DELAY_STEP_MS = 200;
const RETRY_DELAY_CAP_MS = 2000;
const RETRY_MAX_ATTEMPTS = 5;

function retryStrategy(times: number): number | null {
  if (times > RETRY_MAX_ATTEMPTS) return null;
  return Math.min(times * RETRY_DELAY_STEP_MS, RETRY_DELAY_CAP_MS);
}

export function getSessionRedisClient(): Redis | null {
  if (!process.env.OMNIA_SESSION_REDIS_URL && !process.env.OMNIA_SESSION_REDIS_ADDR) {
    return null;
  }

  if (!globalForRedis.sessionRedis) {
    const url = process.env.OMNIA_SESSION_REDIS_URL;
    if (url) {
      globalForRedis.sessionRedis = new Redis(url, {
        connectTimeout: CONNECT_TIMEOUT_MS,
        maxRetriesPerRequest: MAX_RETRIES_PER_REQUEST,
        retryStrategy,
      });
    } else {
      const addr = process.env.OMNIA_SESSION_REDIS_ADDR || "localhost:6379";
      const [host, port] = addr.split(":");
      globalForRedis.sessionRedis = new Redis({
        host,
        port: Number.parseInt(port || "6379", 10),
        password: process.env.OMNIA_SESSION_REDIS_PASSWORD || undefined,
        db: Number.parseInt(process.env.OMNIA_SESSION_REDIS_DB || "0", 10),
        connectTimeout: CONNECT_TIMEOUT_MS,
        maxRetriesPerRequest: MAX_RETRIES_PER_REQUEST,
        retryStrategy,
      });
    }

    globalForRedis.sessionRedis.on("error", (err) => {
      console.error("Session Redis client error", err);
    });
  }

  return globalForRedis.sessionRedis;
}
