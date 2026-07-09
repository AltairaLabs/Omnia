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

// This is a process-lifetime singleton, so retryStrategy MUST NEVER return
// null: returning null tells ioredis to stop reconnecting for good, which
// bricks the client until the pod restarts. If Redis is briefly unavailable
// around startup (e.g. its pod becomes Ready after the dashboard pod), the
// client would give up permanently and every login would fail with
// "Connection is closed" (#1810). Retry forever with capped backoff instead;
// maxRetriesPerRequest keeps individual commands failing fast in the meantime.
function retryStrategy(times: number): number {
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
