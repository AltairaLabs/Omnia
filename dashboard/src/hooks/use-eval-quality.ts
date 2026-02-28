/**
 * Hooks for agent quality dashboard with eval pass rates.
 *
 * useEvalSummary reads from Prometheus (eval gauge metrics).
 * useRecentEvalFailures is a placeholder — the session-api does not yet
 * have an eval-results endpoint, so it returns empty data.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { queryPrometheus, type PrometheusVectorResult } from "@/lib/prometheus";
import { EvalQueries } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import type { EvalResult, EvalResultSummary } from "@/types/eval";

export interface EvalListParams {
  agentName?: string;
  evalId?: string;
  evalType?: string;
  passed?: boolean;
  limit?: number;
  offset?: number;
}

/**
 * Discover eval metric names from Prometheus.
 */
async function discoverEvalMetrics(): Promise<string[]> {
  try {
    const query = EvalQueries.discoverMetrics();
    const resp = await queryPrometheus(query);
    if (resp.status !== "success" || !resp.data?.result) return [];

    const names = new Set<string>();
    for (const item of resp.data.result as PrometheusVectorResult[]) {
      const name = item.metric.__name__;
      if (name && !name.endsWith("_bucket") && !name.endsWith("_sum") && !name.endsWith("_count")) {
        names.add(name);
      }
    }
    return Array.from(names).sort((a, b) => a.localeCompare(b));
  } catch {
    return [];
  }
}

/**
 * Fetch eval summary (pass rates per eval) from Prometheus gauge metrics.
 *
 * Discovers omnia_eval_* metrics, fetches their current values, and
 * transforms them into EvalResultSummary[] for the Overview tab.
 */
export function useEvalSummary() {
  return useQuery({
    queryKey: ["eval-summary"],
    queryFn: async (): Promise<EvalResultSummary[]> => {
      const names = await discoverEvalMetrics();
      if (names.length === 0) return [];

      const results = await Promise.all(
        names.map(async (name) => {
          const resp = await queryPrometheus(name);
          const value =
            resp.status === "success" && resp.data?.result?.[0]?.value
              ? Number.parseFloat(resp.data.result[0].value[1]) || 0
              : 0;

          return {
            evalId: name.replace(/^omnia_eval_/, ""),
            evalType: "metric",
            passRate: value * 100,
            total: 0,
            passed: 0,
            failed: 0,
            avgScore: value,
          } satisfies EvalResultSummary;
        })
      );

      return results;
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

/**
 * Placeholder for recent eval failures.
 *
 * The session-api does not yet expose an eval-results endpoint, so this
 * hook returns empty data. Once the Go backend adds
 * GET /api/v1/eval-results, this can be wired back up.
 */
export function useRecentEvalFailures(_params?: EvalListParams) {
  return useQuery({
    queryKey: ["eval-failures"],
    queryFn: async () => ({ evalResults: [] as EvalResult[], total: 0 }),
    staleTime: DEFAULT_STALE_TIME,
  });
}
