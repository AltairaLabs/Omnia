/**
 * Redis client singleton for the OAuth session store.
 *
 * Backend is gated by OMNIA_SESSION_REDIS_URL: present → returns a
 * client; absent → returns null (callers fall back to MemorySessionStore).
 * URL accepts redis:// or rediss:// (TLS, ACL user, password, DB index
 * all encoded). For password-from-Secret, the chart wires the env from
 * a Kubernetes secretKeyRef so the URL arrives complete at startup.
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
  const url = process.env.OMNIA_SESSION_REDIS_URL;
  if (!url) return null;
  if (!globalForRedis.sessionRedis) {
    globalForRedis.sessionRedis = new Redis(url, {
      connectTimeout: CONNECT_TIMEOUT_MS,
      maxRetriesPerRequest: MAX_RETRIES_PER_REQUEST,
      retryStrategy,
    });
    globalForRedis.sessionRedis.on("error", (err) => {
      console.error("Session Redis client error", err);
    });
  }
  return globalForRedis.sessionRedis;
}
