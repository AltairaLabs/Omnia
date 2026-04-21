/**
 * Redis-backed `SessionStore` for production.
 *
 * Keys are prefixed (`omnia:sess:<sid>`, `omnia:pkce:<state>`) so that a
 * shared Redis instance can host multiple Omnia deployments without
 * collision. `consumePkce` uses `GETDEL` (Redis 6.2+) for atomic
 * single-use semantics — required to prevent PKCE replay.
 */

import type Redis from "ioredis";
import type { PkceRecord, SessionRecord, SessionStore } from "./types";

const SESSION_PREFIX = "omnia:sess:";
const PKCE_PREFIX = "omnia:pkce:";

function requirePositiveTtl(ttlSeconds: number): void {
  if (!Number.isFinite(ttlSeconds) || ttlSeconds <= 0) {
    throw new Error(`ttlSeconds must be > 0, got ${ttlSeconds}`);
  }
}

function parseJson<T>(raw: string | null, context: string): T | null {
  if (raw === null) return null;
  try {
    return JSON.parse(raw) as T;
  } catch (err) {
    console.error(`Session store: failed to parse ${context}`, err);
    return null;
  }
}

export class RedisSessionStore implements SessionStore {
  constructor(private readonly redis: Redis) {}

  async getSession(sid: string): Promise<SessionRecord | null> {
    const raw = await this.redis.get(SESSION_PREFIX + sid);
    return parseJson<SessionRecord>(raw, "session");
  }

  async putSession(sid: string, record: SessionRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    await this.redis.set(SESSION_PREFIX + sid, JSON.stringify(record), "EX", ttlSeconds);
  }

  async deleteSession(sid: string): Promise<void> {
    await this.redis.del(SESSION_PREFIX + sid);
  }

  async putPkce(state: string, record: PkceRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    await this.redis.set(PKCE_PREFIX + state, JSON.stringify(record), "EX", ttlSeconds);
  }

  async consumePkce(state: string): Promise<PkceRecord | null> {
    const raw = await this.redis.getdel(PKCE_PREFIX + state);
    return parseJson<PkceRecord>(raw, "pkce");
  }
}
