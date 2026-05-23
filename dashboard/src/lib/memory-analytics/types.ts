/**
 * Shared types for the /memory-analytics page. Mirrors the AggregateRow /
 * AggregateOptions shapes from internal/memory/stats.go.
 */

export type Tier = "institutional" | "agent" | "user" | "user_for_agent";

export const TIERS: readonly Tier[] = [
  "institutional",
  "agent",
  "user",
  "user_for_agent",
] as const;

export type AggregateGroupBy = "category" | "agent" | "day" | "tier";

export type AggregateMetric = "count" | "distinct_users";

export interface AggregateRow {
  key: string;
  value: number;
  count: number;
}

export interface MemoryAggregateOptions {
  groupBy: AggregateGroupBy;
  metric?: AggregateMetric;
  from?: string;
  to?: string;
  limit?: number;
}

export interface ConsentStats {
  totalUsers: number;
  optedOutAll: number;
  grantsByCategory: Record<string, number>;
}

export function isTier(key: string): key is Tier {
  return TIERS.includes(key as Tier);
}
