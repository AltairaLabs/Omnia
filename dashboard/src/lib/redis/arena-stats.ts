/**
 * Reads arena job statistics from Redis accumulator hashes.
 *
 * Redis key patterns (from ee/pkg/arena/queue/redis.go):
 * - arena:job:{jobID}:stats            — main stats hash
 * - arena:job:{jobID}:stats:provider:{id} — per-provider stats
 *
 * Hash fields: total, passed, failed, totalDurationMs, totalTokens, totalCost
 */

import type Redis from "ioredis";

// ---- Public types ----

export interface ProviderLiveStats {
  total: number;
  passed: number;
  failed: number;
  avgLatencyMs: number;
  totalTokens: number;
  totalCost: number;
}

export interface ArenaLiveStats {
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  avgLatencyMs: number;
  totalTokens: number;
  totalCost: number;
  errorRate: number;
  byProvider: Record<string, ProviderLiveStats>;
}

// ---- Redis key constants (mirror Go) ----

const JOB_KEY_PREFIX = "arena:job:";
const STATS_SUFFIX = ":stats";
const PROVIDER_INFIX = ":stats:provider:";

// ---- Hash field names ----

const FIELD_TOTAL = "total";
const FIELD_PASSED = "passed";
const FIELD_FAILED = "failed";
const FIELD_TOTAL_DURATION = "totalDurationMs";
const FIELD_TOTAL_TOKENS = "totalTokens";
const FIELD_TOTAL_COST = "totalCost";

// ---- Helpers ----

function safeInt(val: string | undefined): number {
  if (!val) return 0;
  const n = Number.parseInt(val, 10);
  return Number.isNaN(n) ? 0 : n;
}

function safeFloat(val: string | undefined): number {
  if (!val) return 0;
  const n = Number.parseFloat(val);
  return Number.isNaN(n) ? 0 : n;
}

function computeAvgLatency(totalDurationMs: number, total: number): number {
  if (total === 0) return 0;
  return Math.round(totalDurationMs / total);
}

function computeRate(numerator: number, denominator: number): number {
  if (denominator === 0) return 0;
  return numerator / denominator;
}

/** Parse a Redis hash map into a ProviderLiveStats object. */
export function parseGroupHash(data: Record<string, string>): ProviderLiveStats {
  const total = safeInt(data[FIELD_TOTAL]);
  const totalDurationMs = safeFloat(data[FIELD_TOTAL_DURATION]);
  return {
    total,
    passed: safeInt(data[FIELD_PASSED]),
    failed: safeInt(data[FIELD_FAILED]),
    avgLatencyMs: computeAvgLatency(totalDurationMs, total),
    totalTokens: safeInt(data[FIELD_TOTAL_TOKENS]),
    totalCost: safeFloat(data[FIELD_TOTAL_COST]),
  };
}

/** Parse a Redis hash map into top-level ArenaLiveStats (without byProvider). */
export function parseMainHash(data: Record<string, string>): Omit<ArenaLiveStats, "byProvider"> {
  const total = safeInt(data[FIELD_TOTAL]);
  const passed = safeInt(data[FIELD_PASSED]);
  const failed = safeInt(data[FIELD_FAILED]);
  const totalDurationMs = safeFloat(data[FIELD_TOTAL_DURATION]);
  return {
    total,
    passed,
    failed,
    passRate: computeRate(passed, total),
    avgLatencyMs: computeAvgLatency(totalDurationMs, total),
    totalTokens: safeInt(data[FIELD_TOTAL_TOKENS]),
    totalCost: safeFloat(data[FIELD_TOTAL_COST]),
    errorRate: computeRate(failed, total),
  };
}

/**
 * Read live arena stats for a job from Redis.
 * Returns null if no stats exist yet (job hasn't started producing results).
 */
export async function readArenaStats(
  redis: Redis,
  jobID: string
): Promise<ArenaLiveStats | null> {
  const mainKey = JOB_KEY_PREFIX + jobID + STATS_SUFFIX;
  const mainData = await redis.hgetall(mainKey);

  // No data yet — job hasn't produced any results
  if (!mainData || Object.keys(mainData).length === 0) {
    return null;
  }

  const stats: ArenaLiveStats = {
    ...parseMainHash(mainData),
    byProvider: {},
  };

  // Scan for provider sub-keys
  const providerPattern = JOB_KEY_PREFIX + jobID + PROVIDER_INFIX + "*";
  const prefixLen = (JOB_KEY_PREFIX + jobID + PROVIDER_INFIX).length;

  let cursor = "0";
  do {
    const [nextCursor, keys] = await redis.scan(cursor, "MATCH", providerPattern, "COUNT", 100);
    cursor = nextCursor;

    for (const key of keys) {
      const providerID = key.slice(prefixLen);
      const providerData = await redis.hgetall(key);
      if (providerData && Object.keys(providerData).length > 0) {
        stats.byProvider[providerID] = parseGroupHash(providerData);
      }
    }
  } while (cursor !== "0");

  return stats;
}
